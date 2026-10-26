// Package server serves goged's browser to web browsers: every browser tab
// gets a model of its own, as a terminal would run it, whose screens the
// page draws and to which it sends keys and the mouse, over a WebSocket.
//
// The models share the document and what is worked out from it
// (tui.Shared). Nothing about the file leaves the server before the page
// has given the secret token: the page, its script, style, font and icon
// are the same for every file, and on the WebSocket the page speaks first,
// with the token. The page loads nothing from anywhere else.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/fonts"
	"github.com/efi/goged/tui"
	"github.com/muesli/termenv"
)

//go:embed static
var static embed.FS

// Config sets up a server.
type Config struct {
	// Shared is the document, as loaded at the start.
	Shared *tui.Shared
	// Options are those of every model. With Path and Load the models
	// watch the file and offer to load it again; the server loads each
	// version once for all of them. Save is the server's.
	Options tui.Options
	// Token is the secret the page must give first; empty makes one.
	Token string
	// Logf, if set, reports refused connections.
	Logf func(format string, args ...any)
}

// Limits.
const (
	maxMessage   = 1 << 16          // the largest message from the page
	helloTimeout = 10 * time.Second // for the page's first message
	readTimeout  = 75 * time.Second // the page answers the pings in time
	pingInterval = 30 * time.Second
	idleTimeout  = 15 * time.Minute // a session without a page lasts so long
	maxSessions  = 64
)

// Close codes of the server.
const (
	closeWrongToken  = 4001 // the page gave a wrong token
	closeTakenOver   = 4002 // the session was opened in another tab
	closeTooMany     = 4003 // too many sessions
	closeSessionOver = 4004 // the user quit, or the session expired
)

// Server serves the browser.
type Server struct {
	opts    tui.Options
	token   string
	load    func() (*tui.Shared, error)
	logf    func(format string, args ...any)
	idle    time.Duration
	penalty time.Duration // the wait before a wrong token is refused

	loadMu   sync.Mutex
	shared   *tui.Shared // the newest version loaded
	sharedAt fileState   // its file's state when it was loaded

	mu       sync.Mutex
	sessions map[string]*session
}

// fileState tells versions of a file apart.
type fileState struct {
	mod  int64
	size int64
}

func statFile(path string) (fileState, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return fileState{}, false
	}
	return fileState{fi.ModTime().UnixNano(), fi.Size()}, true
}

// New makes a server. As the screens go to browsers, it has lipgloss
// render colors in full and for a dark background, for the whole
// program.
func New(cfg Config) *Server {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	s := &Server{
		opts:     cfg.Options,
		token:    cfg.Token,
		load:     cfg.Options.Load,
		logf:     cfg.Logf,
		idle:     idleTimeout,
		penalty:  time.Second,
		shared:   cfg.Shared,
		sessions: map[string]*session{},
	}
	if s.token == "" {
		s.token = randomID()
	}
	if s.logf == nil {
		s.logf = func(string, ...any) {}
	}
	if s.opts.Path != "" {
		s.sharedAt, _ = statFile(s.opts.Path)
	}
	if s.load != nil {
		s.opts.Load = s.loadShared
	}
	s.opts.NoMouse = false
	return s
}

// Token returns the secret the page must give.
func (s *Server) Token() string { return s.token }

// randomID returns 128 random bits in hex.
func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// loadShared returns the file as it is now: the version loaded before if
// the file has not changed since, so that all models share it.
func (s *Server) loadShared() (*tui.Shared, error) {
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	st, ok := statFile(s.opts.Path)
	if ok && st == s.sharedAt && s.shared != nil {
		return s.shared, nil
	}
	sh, err := s.load()
	if err != nil {
		return nil, err
	}
	s.shared, s.sharedAt = sh, st
	return sh, nil
}

// current returns the newest version of the file for a new session.
func (s *Server) current() *tui.Shared {
	if s.load != nil && s.opts.Path != "" {
		if sh, err := s.loadShared(); err == nil {
			return sh
		}
	}
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	return s.shared
}

// Handler returns the server's HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	file := func(name, typ string, data func() ([]byte, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			b, err := data()
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", typ)
			if name == "index.html" {
				w.Header().Set("Cache-Control", "no-store")
			}
			_, _ = w.Write(b)
		}
	}
	embedded := func(name string) func() ([]byte, error) {
		return func() ([]byte, error) { return static.ReadFile("static/" + name) }
	}
	fixed := func(b []byte) func() ([]byte, error) { return func() ([]byte, error) { return b, nil } }
	mux.Handle("GET /{$}", file("index.html", "text/html; charset=utf-8", embedded("index.html")))
	mux.Handle("GET /goged.js", file("goged.js", "text/javascript; charset=utf-8", embedded("goged.js")))
	mux.Handle("GET /goged.css", file("goged.css", "text/css; charset=utf-8", embedded("goged.css")))
	mux.Handle("GET /favicon.png", file("favicon.png", "image/png", embedded("favicon.png")))
	mux.Handle("GET /fonts/MartianMono-CnRg.ttf", file("", "font/ttf", fixed(fonts.Regular)))
	mux.Handle("GET /fonts/MartianMono-CnBd.ttf", file("", "font/ttf", fixed(fonts.Bold)))
	mux.HandleFunc("GET /ws", s.serveWS)
	return securityHeaders(mux)
}

// securityHeaders keeps the page to this server: it loads and connects to
// nothing else, and no other site can frame it.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; "+
			"img-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether a WebSocket request comes from a page of this
// server. Browsers send Origin with every WebSocket handshake; other
// programs need the token all the same.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host
}

// hello is the page's first message.
type hello struct {
	Token   string `json:"token"`
	Session string `json:"session"`
	Cols    int    `json:"cols"`
	Rows    int    `json:"rows"`
}

// serveWS serves a page's connection: the token first, then the session.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		s.logf("goged: refused a connection from %s for the page of %s", r.RemoteAddr, r.Header.Get("Origin"))
		http.Error(w, "this page is not one of this server's", http.StatusForbidden)
		return
	}
	c, err := upgrade(w, r, maxMessage)
	if err != nil {
		return
	}
	// The page speaks first; until the token is right nothing is sent.
	c.readTimeout = helloTimeout
	op, data, err := c.readMessage()
	var h hello
	if err != nil || op != opText || json.Unmarshal(data, &h) != nil ||
		subtle.ConstantTimeCompare([]byte(h.Token), []byte(s.token)) != 1 {
		s.logf("goged: refused %s: no or a wrong token", r.RemoteAddr)
		time.Sleep(s.penalty)
		c.Close(closeWrongToken, "")
		return
	}
	c.readTimeout = readTimeout
	ss := s.session(h.Session)
	if ss == nil {
		c.Close(closeTooMany, "too many sessions")
		return
	}
	if !ss.post(attachEvent{c, h.Cols, h.Rows}) {
		c.Close(closeSessionOver, "") // it ended just now
		return
	}
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if c.ping() != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}()
	ss.read(c)
	close(stop)
	ss.post(detachEvent{c})
}

// session returns the session with id, or a new one; nil if there are
// too many.
func (s *Server) session(id string) *session {
	s.mu.Lock()
	if ss, ok := s.sessions[id]; ok && !ss.over() {
		s.mu.Unlock()
		return ss
	}
	if len(s.sessions) >= maxSessions {
		s.mu.Unlock()
		return nil
	}
	ss := newSession(s, randomID())
	s.sessions[ss.id] = ss
	s.mu.Unlock()
	ss.start(s.current())
	return ss
}

// ended forgets a session.
func (s *Server) ended(ss *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[ss.id] == ss {
		delete(s.sessions, ss.id)
	}
}

// Sessions returns the number of sessions.
func (s *Server) Sessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}
