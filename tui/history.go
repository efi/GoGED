package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

// history keeps what was used in a text input during the session, the
// search query or a filter: alt+↑ steps back through it, alt+↓ forward,
// and forward past the newest to a new empty line.
type history struct {
	entries []string // the oldest first
	pos     int      // the entry shown; len(entries) is the new line
}

// maxHistory is how many entries a history keeps.
const maxHistory = 100

// add records text as the newest entry, unless it is blank; an earlier
// entry with the same text moves to the end. The next step back starts
// from the newest.
func (h *history) add(text string) {
	if strings.TrimSpace(text) != "" {
		h.entries = slices.DeleteFunc(h.entries, func(e string) bool { return e == text })
		h.entries = append(h.entries, text)
		if len(h.entries) > maxHistory {
			h.entries = h.entries[len(h.entries)-maxHistory:]
		}
	}
	h.pos = len(h.entries)
}

// step moves back (dir < 0) or forward from text, the content of the
// input, and returns what the input shows next; ok is false where there
// is nothing to move to. Text that was typed or changed is first kept as
// the newest entry, so stepping never loses it.
func (h *history) step(dir int, text string) (next string, ok bool) {
	if strings.TrimSpace(text) != "" && (h.pos >= len(h.entries) || h.entries[h.pos] != text) {
		h.add(text)
		h.pos = len(h.entries) - 1
	}
	switch pos := h.pos + max(-1, min(1, dir)); {
	case pos < 0:
		return text, false
	case pos >= len(h.entries):
		h.pos = len(h.entries)
		return "", text != ""
	default:
		h.pos = pos
		return h.entries[pos], true
	}
}

// recall steps through a history from the content of an input, and puts
// the entry in the input. It reports whether the content changed.
func recall(h *history, in *textinput.Model, dir int) bool {
	text, ok := h.step(dir, in.Value())
	if !ok {
		return false
	}
	in.SetValue(text)
	in.CursorEnd()
	return true
}

// historyKey returns the direction of a key that steps through a history,
// or 0.
func historyKey(key string) int {
	switch key {
	case "alt+up":
		return -1
	case "alt+down":
		return 1
	}
	return 0
}
