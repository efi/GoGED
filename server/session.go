package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/token"
	"io"
	"mime"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/tui"
)

// A session is one browser tab's goged: a model, run as Bubble Tea would
// run it. One goroutine owns the model and the page's connection; keys,
// the mouse, the results of commands and the page coming and going reach
// it as events. After every burst of events it sends the lines of the
// screen that changed. A session outlives its page for a while, so that a
// reloaded page carries on where it was.
type session struct {
	id     string
	srv    *Server
	events chan any
	done   chan struct{}

	// Owned by run.
	model      tea.Model
	conn       *wsConn
	cols, rows int
	last       []string // the lines the page shows
	title      string
}

// Events besides Bubble Tea's messages.
type (
	attachEvent struct {
		c          *wsConn
		cols, rows int
	}
	detachEvent struct{ c *wsConn }
	sizeEvent   struct{ cols, rows int }
	titleEvent  string
	quitEvent   struct{}
)

func newSession(s *Server, id string) *session {
	return &session{id: id, srv: s, events: make(chan any, 64), done: make(chan struct{}), cols: 80, rows: 24}
}

// start makes the model and runs the session.
func (ss *session) start(sh *tui.Shared) {
	opts := ss.srv.opts
	opts.Save = ss.save
	ss.model = tui.NewModel(sh, opts)
	ss.title = "goged"
	if opts.Title != "" {
		ss.title += " — " + opts.Title
	}
	go ss.run()
}

// post hands the session an event; false if it has ended.
func (ss *session) post(ev any) bool {
	select {
	case ss.events <- ev:
		return true
	case <-ss.done:
		return false
	}
}

// over reports whether the session has ended.
func (ss *session) over() bool {
	select {
	case <-ss.done:
		return true
	default:
		return false
	}
}

func (ss *session) run() {
	defer ss.end()
	idle := time.NewTimer(ss.srv.idle)
	defer idle.Stop()
	ss.exec(ss.model.Init())
	for {
		select {
		case ev := <-ss.events:
			if !ss.handle(ev, idle) {
				return
			}
			// What else has come is handled before the screen is drawn.
			for more := true; more; {
				select {
				case ev := <-ss.events:
					if !ss.handle(ev, idle) {
						return
					}
				default:
					more = false
				}
			}
			ss.draw()
		case <-idle.C:
			if ss.conn == nil {
				return
			}
		}
	}
}

// handle handles an event; false ends the session.
func (ss *session) handle(ev any, idle *time.Timer) bool {
	switch ev := ev.(type) {
	case attachEvent:
		if ss.conn != nil && ss.conn != ev.c {
			ss.conn.Close(closeTakenOver, "the session was opened in another tab")
		}
		ss.conn, ss.last = ev.c, nil
		idle.Stop()
		ss.send(map[string]any{"t": "hello", "session": ss.id, "title": ss.title})
		ss.resize(ev.cols, ev.rows)
	case detachEvent:
		if ss.conn == ev.c {
			ss.conn = nil
			idle.Reset(ss.srv.idle)
		}
	case sizeEvent:
		ss.resize(ev.cols, ev.rows)
	case mouseInput:
		if msg, ok := mouseMsg(mouseEvent(ev), ss.cols, ss.rows); ok {
			ss.update(msg)
		}
	case titleEvent:
		ss.title = string(ev)
		ss.send(map[string]any{"t": "title", "title": ss.title})
	case quitEvent:
		ss.send(map[string]any{"t": "bye"})
		if ss.conn != nil {
			ss.conn.Close(closeSessionOver, "")
		}
		return false
	default:
		ss.update(ev)
	}
	return true
}

// resize gives the model the page's size and has the whole screen sent.
func (ss *session) resize(cols, rows int) {
	ss.cols, ss.rows = max(20, min(cols, 500)), max(5, min(rows, 200))
	ss.last = nil
	ss.update(tea.WindowSizeMsg{Width: ss.cols, Height: ss.rows})
}

func (ss *session) update(msg tea.Msg) {
	var cmd tea.Cmd
	ss.model, cmd = ss.model.Update(msg)
	ss.exec(cmd)
}

// end forgets the session and lets the commands still running go.
func (ss *session) end() {
	close(ss.done)
	ss.srv.ended(ss)
	if ss.conn != nil {
		ss.conn.Close(closeSessionOver, "")
	}
}

// exec runs a command as Bubble Tea would, and hands its message back.
func (ss *session) exec(cmd tea.Cmd) {
	if cmd != nil {
		// Commands wait (a tick waits a second): never in run.
		go func() { ss.deliver(cmd()) }()
	}
}

// teaPackage is the package of Bubble Tea's messages.
var teaPackage = reflect.TypeOf(tea.QuitMsg{}).PkgPath()

// deliver hands a command's message to the session. Batches run at once,
// sequences one after the other; of Bubble Tea's own messages for the
// terminal, only the window title matters here.
func (ss *session) deliver(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, cmd := range msg {
			ss.exec(cmd)
		}
		return
	case tea.QuitMsg:
		ss.post(quitEvent{})
		return
	}
	v := reflect.ValueOf(msg)
	if t := v.Type(); t.PkgPath() == teaPackage && !token.IsExported(t.Name()) {
		switch {
		case t.Name() == "sequenceMsg" && v.CanConvert(reflect.TypeFor[[]tea.Cmd]()):
			for _, cmd := range v.Convert(reflect.TypeFor[[]tea.Cmd]()).Interface().([]tea.Cmd) {
				if cmd != nil {
					ss.deliver(cmd())
				}
			}
		case t.Name() == "setWindowTitleMsg" && v.Kind() == reflect.String:
			ss.post(titleEvent(v.String()))
		}
		return
	}
	ss.post(msg)
}

// read reads the page's messages until the connection ends.
func (ss *session) read(c *wsConn) {
	for {
		op, data, err := c.readMessage()
		if err != nil {
			c.Close(closeGoingAway, "")
			return
		}
		if op != opText {
			continue
		}
		var m struct {
			T    string `json:"t"`
			Key  string `json:"key"`
			Text string `json:"text"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
			mouseEvent
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.T {
		case "key":
			if k, ok := keyMsg(m.Key); ok {
				ss.post(k)
			}
		case "paste":
			if m.Text != "" {
				ss.post(pasteMsg(m.Text))
			}
		case "mouse":
			ss.post(mouseInput(m.mouseEvent))
		case "size":
			ss.post(sizeEvent{m.Cols, m.Rows})
		}
	}
}

// mouseInput is a mouse event from the page, turned into Bubble Tea's
// message by the session, which knows the size of the screen.
type mouseInput mouseEvent

// draw sends the lines of the screen that changed.
func (ss *session) draw() {
	if ss.conn == nil {
		return
	}
	lines := strings.Split(ss.model.View(), "\n")
	lines = lines[:min(len(lines), ss.rows)]
	for len(lines) < ss.rows {
		lines = append(lines, "")
	}
	var changed []any
	for i, l := range lines {
		if ss.last == nil || ss.last[i] != l {
			changed = append(changed, []any{i, lineRuns(l)})
		}
	}
	ss.last = lines
	if len(changed) > 0 {
		ss.send(map[string]any{"t": "frame", "cols": ss.cols, "rows": ss.rows, "lines": changed})
	}
}

// send sends a message to the page, if there is one.
func (ss *session) send(v any) {
	if ss.conn == nil {
		return
	}
	data, err := json.Marshal(v)
	if err == nil {
		err = ss.conn.writeText(data)
	}
	if err != nil {
		ss.conn.Close(closeGoingAway, "")
		ss.conn = nil
	}
}

// save sends a file the user exported or printed to the page, which has
// the browser save it. It runs within the model's Update, in run.
func (ss *session) save(name string, write func(io.Writer) error) error {
	if ss.conn == nil {
		return errors.New("the browser is not connected")
	}
	var b bytes.Buffer
	if err := write(&b); err != nil {
		return err
	}
	typ := mime.TypeByExtension(filepath.Ext(name))
	if typ == "" {
		typ = "application/octet-stream"
	}
	ss.send(map[string]any{"t": "download", "name": name, "type": typ, "size": b.Len()})
	if ss.conn == nil {
		return errors.New("the browser is not connected")
	}
	return ss.conn.writeBinary(b.Bytes())
}
