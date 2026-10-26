package server

import (
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLineRuns(t *testing.T) {
	for _, tt := range []struct {
		line, want string
	}{
		{"plain", `[["plain","","",0]]`},
		{"", `[]`},
		{"\x1b[1;38;2;185;162;255mgoged\x1b[0m · x", `[["goged","#b9a2ff","",1],[" · x","","",0]]`},
		{"\x1b[7m▸ sel\x1b[m", `[["▸ sel","","",16]]`},
		{"\x1b[48;2;1;2;3m\x1b[38;5;196mA\x1b[39mB\x1b[49mC", `[["A","#ff0000","#010203",0],["B","","#010203",0],["C","","",0]]`},
		{"\x1b[31;1mr\x1b[22;4mu\x1b[24;2mf\x1b[0;9ms", `[["r","#cd0000","",1],["u","#cd0000","",8],["f","#cd0000","",2],["s","","",32]]`},
		// Styles that change nothing make no new run.
		{"a\x1b[0mb\x1b[39;49mc", `[["abc","","",0]]`},
		// Wide characters and graphemes of several code points take runs of
		// their own, with their width.
		{"漢字 x", `[["漢","","",0,2],["字","","",0,2],[" x","","",0]]`},
		{"Ame\u0301lie", "[[\"Am\",\"\",\"\",0],[\"e\u0301\",\"\",\"\",0,1],[\"lie\",\"\",\"\",0]]"},
		{"漢\u0301", "[[\"漢\u0301\",\"\",\"\",0,2]]"},
		// Other sequences and control characters take no cell.
		{"a\x1b]0;title\x07b\x1b[2Kc\td", `[["abcd","","",0]]`},
		{"⣿⠁\x1b[1m━╸", `[["⣿⠁","","",0],["━╸","","",1]]`},
	} {
		got, _ := json.Marshal(lineRuns(tt.line))
		if string(got) != tt.want {
			t.Errorf("%q:\n got %s\nwant %s", tt.line, got, tt.want)
		}
	}
}

func TestPalette(t *testing.T) {
	for n, want := range map[int]string{0: "#000000", 9: "#ff0000", 16: "#000000", 21: "#0000ff", 196: "#ff0000", 231: "#ffffff", 232: "#080808", 255: "#eeeeee"} {
		if got := palette(n); got != want {
			t.Errorf("palette(%d) = %s, want %s", n, got, want)
		}
	}
}

// The names the page sends become the keys a terminal brings.
func TestKeyNames(t *testing.T) {
	for _, name := range []string{"up", "down", "enter", "esc", "tab", "shift+tab", "backspace", "delete", "home", "end",
		"pgup", "pgdown", "ctrl+u", "ctrl+d", "ctrl+home", "ctrl+shift+up", "shift+left", "f1", "alt+up", "alt+left",
		"a", "G", "?", "/", "+", " ", "ä", "alt+G", "alt+m", "alt+1", "alt++"} {
		k, ok := keyMsg(name)
		if !ok || k.String() != name {
			t.Errorf("%q: %v %q", name, ok, k.String())
		}
	}
	if k, _ := keyMsg(" "); k.Type != tea.KeySpace {
		t.Errorf("space is %v", k.Type)
	}
	for _, name := range []string{"", "nonsense", "alt+", "ctrl+v", "alt+ctrl+v"} {
		if k, ok := keyMsg(name); ok {
			t.Errorf("%q is %q", name, k.String())
		}
	}
	// Pasted text is no key: it does not match shortcuts.
	if p := pasteMsg("q"); p.String() != "[q]" || !p.Paste {
		t.Errorf("paste: %q", p.String())
	}
}

func TestMouseMsg(t *testing.T) {
	m, ok := mouseMsg(mouseEvent{X: 120, Y: -3, Button: "wheeldown", Action: "press", Ctrl: true}, 100, 30)
	if !ok || m.X != 99 || m.Y != 0 || m.Button != tea.MouseButtonWheelDown || m.Action != tea.MouseActionPress || !m.Ctrl {
		t.Errorf("%+v %v", m, ok)
	}
	for _, e := range []mouseEvent{{Button: "fifth", Action: "press"}, {Button: "left", Action: "hover"}} {
		if _, ok := mouseMsg(e, 100, 30); ok {
			t.Errorf("%+v accepted", e)
		}
	}
}
