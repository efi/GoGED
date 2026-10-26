package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/efi/goged/fonts"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/tui"
)

const testToken = "s3cret-t0ken"

func sample(t *testing.T) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func newTestServer(t *testing.T, opts tui.Options) (*Server, *httptest.Server) {
	t.Helper()
	if opts.Title == "" {
		opts.Title = "family.ged"
	}
	s := New(Config{Shared: tui.NewShared(sample(t)), Options: opts, Token: testToken})
	s.penalty = 0
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// page is a browser tab for the tests: it keeps the screen as text.
type page struct {
	*wsClient
	session string
	title   string
	screen  []string
	files   map[string][]byte
	bye     bool
}

// openPage connects, gives the token and carries on session, if any.
func openPage(t *testing.T, srv *httptest.Server, session string) *page {
	t.Helper()
	c, resp := dialWS(t, srv, "/ws", "Origin", srv.URL)
	if c == nil {
		t.Fatalf("handshake: %s", resp.Status)
	}
	p := &page{wsClient: c, files: map[string][]byte{}}
	p.sendJSON(map[string]any{"token": testToken, "session": session, "cols": 100, "rows": 30})
	p.until(func() bool { return p.session != "" && len(p.screen) == 30 })
	return p
}

func (p *page) sendJSON(v any) {
	data, _ := json.Marshal(v)
	p.send(string(data))
}

func (p *page) key(keys ...string) {
	for _, k := range keys {
		p.sendJSON(map[string]any{"t": "key", "key": k})
	}
}

// next reads a message and applies it; false when the connection ends.
func (p *page) next(within time.Duration) (bool, error) {
	op, data, err := p.read(within)
	if err != nil {
		return false, err
	}
	switch op {
	case opClose:
		return false, nil
	case opBinary:
		return true, errors.New("a file came unannounced")
	}
	var m struct {
		T, Session, Title, Name string
		Rows                    int
		Lines                   [][]json.RawMessage
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return false, err
	}
	switch m.T {
	case "hello":
		p.session, p.title = m.Session, m.Title
	case "frame":
		for len(p.screen) < m.Rows {
			p.screen = append(p.screen, "")
		}
		p.screen = p.screen[:m.Rows]
		for _, l := range m.Lines {
			var i int
			var runs [][]any
			_ = json.Unmarshal(l[0], &i)
			_ = json.Unmarshal(l[1], &runs)
			var b strings.Builder
			for _, r := range runs {
				b.WriteString(r[0].(string))
			}
			p.screen[i] = b.String()
		}
	case "download":
		op, data, err := p.read(time.Second)
		if err != nil || op != opBinary {
			return false, errors.New("no file after the announcement")
		}
		p.files[m.Name] = data
	case "bye":
		p.bye = true
	}
	return true, nil
}

// until applies messages until ok holds.
func (p *page) until(ok func() bool) {
	p.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out; the screen:\n%s", p.text())
		}
		if more, err := p.next(5 * time.Second); err != nil || !more {
			p.t.Fatalf("connection: %v; the screen:\n%s", err, p.text())
		}
	}
}

func (p *page) text() string { return strings.Join(p.screen, "\n") }

// shows waits until the screen shows all of want.
func (p *page) shows(want ...string) {
	p.t.Helper()
	p.until(func() bool {
		for _, w := range want {
			if !strings.Contains(p.text(), w) {
				return false
			}
		}
		return true
	})
}

// Nothing is sent before the token, and nothing but a close after a
// wrong one.
func TestTokenFirst(t *testing.T) {
	_, srv := newTestServer(t, tui.Options{})

	c, _ := dialWS(t, srv, "/ws", "Origin", srv.URL)
	if _, _, err := c.read(300 * time.Millisecond); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("before the token: %v", err)
	}
	c.send(`{"token":"guess","cols":80,"rows":24}`)
	if code := c.closeCode(); code != closeWrongToken {
		t.Errorf("wrong token: close %d", code)
	}

	for _, first := range []string{`not json`, `{"session":"x"}`} {
		c, _ = dialWS(t, srv, "/ws", "Origin", srv.URL)
		c.send(first)
		op, _, err := c.read(time.Second)
		if err != nil || op != opClose {
			t.Errorf("%s: op %d, %v", first, op, err)
		}
	}
	c, _ = dialWS(t, srv, "/ws", "Origin", srv.URL)
	c.frame(true, opBinary, []byte(`{"token":"`+testToken+`"}`))
	if code := c.closeCode(); code != closeWrongToken {
		t.Errorf("token in a binary message: close %d", code)
	}

	p := openPage(t, srv, "")
	if p.title != "goged — family.ged" || len(p.session) != 32 {
		t.Errorf("hello: %q %q", p.title, p.session)
	}
	p.shows("goged", "family.ged · 24 people · 9 families", "[1 Search]")

	// Pages of other sites are turned away.
	if c, resp := dialWS(t, srv, "/ws", "Origin", "http://evil.example"); c != nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("another site's page: %v", resp.Status)
	}
}

func TestSession(t *testing.T) {
	_, srv := newTestServer(t, tui.Options{})
	p := openPage(t, srv, "")
	p.key("i", "d", ":", "I", "3", "enter")
	p.shows("John Smith", "male · I3 · 1817–1880", "Ann Taylor")

	// alt+G, and a click on a tab.
	p.key("alt+G")
	p.shows("names are colored by sex")
	col := strings.Index(p.screen[1], "4 Events")
	p.sendJSON(map[string]any{"t": "mouse", "x": col + 1, "y": 1, "button": "left", "action": "press"})
	p.sendJSON(map[string]any{"t": "mouse", "x": col + 1, "y": 1, "button": "left", "action": "release"})
	p.shows("[4 Events]", "Marriage")

	// Pasted text is typed; ctrl+v would paste the server's clipboard.
	p.key("/", "ctrl+u")
	p.sendJSON(map[string]any{"t": "paste", "text": "Arthur"})
	p.shows("Search: Arthur", "Smith, Arthur")
	p.key("ctrl+v")

	// A smaller page gets the whole screen at its size.
	p.sendJSON(map[string]any{"t": "size", "cols": 60, "rows": 12})
	p.until(func() bool { return len(p.screen) == 12 })
	p.shows("Smith, Arthur")
	for i, l := range p.screen {
		if w := len([]rune(l)); w > 60 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

// A page that comes back carries on; a second tab takes the session over.
func TestResume(t *testing.T) {
	s, srv := newTestServer(t, tui.Options{})
	p := openPage(t, srv, "")
	p.key("i", "d", ":", "I", "1", "4", "enter", "m")
	p.shows("Arthur Smith", "★")
	p.conn.Close()

	q := openPage(t, srv, p.session)
	if q.session != p.session {
		t.Errorf("session %s, then %s", p.session, q.session)
	}
	q.shows("Arthur Smith", "★ Arthur Smith")

	r := openPage(t, srv, p.session)
	if code := q.closeCode(); code != closeTakenOver {
		t.Errorf("the first tab: close %d", code)
	}
	r.shows("Arthur Smith")
	if n := s.Sessions(); n != 1 {
		t.Errorf("%d sessions", n)
	}

	// An unknown session starts a new one.
	u := openPage(t, srv, "no-such-session")
	if u.session == p.session || u.session == "no-such-session" {
		t.Errorf("session %s", u.session)
	}
}

// Exports and printed charts go to the page.
func TestDownload(t *testing.T) {
	dir := t.TempDir()
	_, srv := newTestServer(t, tui.Options{Path: filepath.Join(dir, "family.ged")})
	p := openPage(t, srv, "")
	p.key("i", "d", ":", "I", "1", "4", "enter", "t", "P")
	p.shows("Print chart", "family-pedigree-I14.pdf")
	p.key("enter")
	p.until(func() bool { return p.files["family-pedigree-I14.pdf"] != nil })
	if !bytes.HasPrefix(p.files["family-pedigree-I14.pdf"], []byte("%PDF")) {
		t.Error("the chart is not a PDF")
	}
	p.shows("for the browser to save")
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("written on the server: %v", entries)
	}
}

// q ends the session; a session without its page ends after a while.
func TestSessionEnds(t *testing.T) {
	s, srv := newTestServer(t, tui.Options{})
	p := openPage(t, srv, "")
	p.key("i", "d", ":", "I", "3", "enter", "q")
	p.until(func() bool { return p.bye })
	if code := p.closeCode(); code != closeSessionOver {
		t.Errorf("close %d", code)
	}
	waitFor(t, func() bool { return s.Sessions() == 0 })

	s.idle = 50 * time.Millisecond
	q := openPage(t, srv, "")
	q.conn.Close()
	waitFor(t, func() bool { return s.Sessions() == 0 })
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
	}
}

// Every version of the file is loaded once for all sessions.
func TestLoadOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "family.ged")
	data, err := os.ReadFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	loads := 0
	load := func() (*tui.Shared, error) {
		loads++
		doc, err := gedcom.ParseFile(path)
		if err != nil {
			return nil, err
		}
		return tui.NewShared(doc), nil
	}
	first, _ := load()
	s := New(Config{Shared: first, Options: tui.Options{Title: "family.ged", Path: path, Load: load}, Token: testToken})
	if a, b := s.current(), s.current(); a != first || b != first || loads != 1 {
		t.Errorf("unchanged: %p %p, first %p, %d loads", a, b, first, loads)
	}
	later := time.Now().Add(time.Minute)
	if err := os.WriteFile(path, append(data, "0 TRLR\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(path, later, later)
	a, _ := s.opts.Load()
	b, _ := s.opts.Load()
	if a == first || a != b || loads != 2 {
		t.Errorf("changed: %p %p, first %p, %d loads", a, b, first, loads)
	}
}

// The page and its files tell nothing about the file shown, and keep to
// this server.
func TestStatic(t *testing.T) {
	_, srv := newTestServer(t, tui.Options{})
	get := func(path string) (*http.Response, []byte) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, body
	}
	for _, f := range []struct{ path, typ, has string }{
		{"/", "text/html; charset=utf-8", `<canvas id="screen"`},
		{"/goged.js", "text/javascript; charset=utf-8", "function connect"},
		{"/goged.css", "text/css; charset=utf-8", "@font-face"},
		{"/favicon.png", "image/png", "\x89PNG"},
		{"/fonts/MartianMono-CnRg.ttf", "font/ttf", string(fonts.Regular[:16])},
		{"/fonts/MartianMono-CnBd.ttf", "font/ttf", string(fonts.Bold[:16])},
	} {
		resp, body := get(f.path)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != f.typ || !strings.Contains(string(body), f.has) {
			t.Errorf("%s: %s %s", f.path, resp.Status, resp.Header.Get("Content-Type"))
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "connect-src 'self'") {
			t.Errorf("%s: CSP %q", f.path, csp)
		}
		if strings.Contains(string(body), "family.ged") || strings.Contains(string(body), "Smith") {
			t.Errorf("%s tells about the file", f.path)
		}
	}
	// Everything the page loads is local.
	for _, f := range []string{"/", "/goged.js", "/goged.css"} {
		_, body := get(f)
		if strings.Contains(string(body), "http://") || strings.Contains(string(body), "https://") || strings.Contains(string(body), "//cdn") {
			t.Errorf("%s refers elsewhere", f)
		}
	}
	if resp, _ := get("/nothing"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("/nothing: %s", resp.Status)
	}
}

// Many pages at once; run with -race.
func TestManyPages(t *testing.T) {
	_, srv := newTestServer(t, tui.Options{})
	done := make(chan *page)
	for i := range 6 {
		go func() {
			p := openPage(t, srv, "")
			keys := [][]string{{"s", "m", "i", "enter", "t"}, {"j", "o", "enter", "alt+6", "M", "n"}, {"alt+8", "down", "enter", "alt+7"}}[i%3]
			p.key(keys...)
			p.key("alt+4")
			p.shows("[4 Events]")
			done <- p
		}()
	}
	for range 6 {
		<-done
	}
}

var _ net.Conn // the test client dials TCP itself

// A watched file: the ticks that look at it hold nothing up, and a new
// version is loaded once for the pages that load it again.
func TestWatchedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "family.ged")
	data, err := os.ReadFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var loads atomic.Int32
	load := func() (*tui.Shared, error) {
		loads.Add(1)
		doc, err := gedcom.ParseFile(path)
		if err != nil {
			return nil, err
		}
		return tui.NewShared(doc), nil
	}
	first, _ := load()
	s := New(Config{Shared: first, Options: tui.Options{Title: "family.ged", Path: path, Load: load}, Token: testToken})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	p, q := openPage(t, srv, ""), openPage(t, srv, "")
	p.key("i", "d", ":", "I", "3", "enter")
	p.shows("John Smith")
	time.Sleep(1200 * time.Millisecond) // a tick or so
	for _, k := range []struct{ key, shows string }{{"t", "Pedigree of"}, {"d", "Descendants of"}, {"h", "Hourglass of"}, {"p", "Pedigree of"}} {
		start := time.Now()
		p.key(k.key)
		p.shows(k.shows)
		if d := time.Since(start); d > 400*time.Millisecond {
			t.Errorf("%s took %v", k.key, d)
		}
	}

	// Another program changes the file; both pages load it again.
	later := time.Now().Add(time.Minute)
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "1 NAME John /Smith/", "1 NAME Johann /Smith/", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(path, later, later)
	for _, x := range []*page{p, q} {
		x.shows("family.ged changed")
		time.Sleep(600 * time.Millisecond) // keys right away were typed for the view
		x.key("y")
	}
	p.shows("Johann Smith")
	q.key("i", "d", ":", "I", "3", "enter")
	q.shows("Johann Smith")
	if n := loads.Load(); n != 2 {
		t.Errorf("%d loads", n)
	}
}
