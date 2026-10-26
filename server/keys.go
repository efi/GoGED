package server

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// The page sends keys by the names Bubble Tea gives them ("up", "ctrl+u",
// "alt+G", "a") and the mouse by its buttons and actions; the server turns
// them into the messages a terminal would have brought.

// keyTypes maps key names to their types, as Bubble Tea names them.
var keyTypes = func() map[string]tea.KeyType {
	m := map[string]tea.KeyType{}
	for t := tea.KeyType(-200); t <= 127; t++ {
		if t == tea.KeyRunes {
			continue
		}
		if name := (tea.Key{Type: t}).String(); name != "" {
			if _, ok := m[name]; !ok {
				m[name] = t
			}
		}
	}
	return m
}()

// noKeys are the keys a browser does not pass on: ctrl+v would paste the
// server's clipboard, not the browser's, which the page sends as a paste.
var noKeys = map[string]bool{"ctrl+v": true, "alt+ctrl+v": true}

// keyMsg returns the key named, or false for an unknown name.
func keyMsg(name string) (tea.KeyMsg, bool) {
	if noKeys[name] {
		return tea.KeyMsg{}, false
	}
	var k tea.Key
	if rest, ok := strings.CutPrefix(name, "alt+"); ok && rest != "" {
		k.Alt, name = true, rest
	}
	switch t, ok := keyTypes[name]; {
	case ok:
		k.Type = t
	case utf8.RuneCountInString(name) == 1:
		k.Type, k.Runes = tea.KeyRunes, []rune(name)
	default:
		return tea.KeyMsg{}, false
	}
	return tea.KeyMsg(k), true
}

// pasteMsg is text pasted in the browser.
func pasteMsg(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true}
}

var mouseButtons = map[string]tea.MouseButton{
	"none": tea.MouseButtonNone, "left": tea.MouseButtonLeft, "middle": tea.MouseButtonMiddle,
	"right": tea.MouseButtonRight, "wheelup": tea.MouseButtonWheelUp, "wheeldown": tea.MouseButtonWheelDown,
	"wheelleft": tea.MouseButtonWheelLeft, "wheelright": tea.MouseButtonWheelRight,
	"backward": tea.MouseButtonBackward, "forward": tea.MouseButtonForward,
}

var mouseActions = map[string]tea.MouseAction{
	"press": tea.MouseActionPress, "release": tea.MouseActionRelease, "motion": tea.MouseActionMotion,
}

// mouseEvent is a mouse event as the page sends it: the cell and what
// happened there.
type mouseEvent struct {
	X, Y             int
	Button, Action   string
	Alt, Ctrl, Shift bool
}

// mouseMsg returns the mouse event as Bubble Tea's message, or false if it
// names an unknown button or action.
func mouseMsg(e mouseEvent, cols, rows int) (tea.MouseMsg, bool) {
	b, okB := mouseButtons[e.Button]
	a, okA := mouseActions[e.Action]
	if !okB || !okA {
		return tea.MouseMsg{}, false
	}
	return tea.MouseMsg{
		X: max(0, min(e.X, cols-1)), Y: max(0, min(e.Y, rows-1)),
		Button: b, Action: a, Alt: e.Alt, Ctrl: e.Ctrl, Shift: e.Shift,
	}, true
}
