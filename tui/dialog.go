package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// A dialog is a small question over the view, answered with one of its
// buttons: whether to load the file again, or what to show of a surname.

// dialogGrace is how long a dialog that comes unasked ignores keys: they
// were meant for the view.
const dialogGrace = 500 * time.Millisecond

type dialog struct {
	title   string
	text    []string // the question, a line each
	choices []string
	keys    map[string]int // keys that choose at once
	cancel  int            // what esc chooses, or -1: nothing
	hints   [][2]string    // the keys for the footer
	choice  int
	since   time.Time // when an unasked dialog appeared; zero for one asked for
	answer  func(m *Model, choice int) tea.Cmd
}

// ask shows a dialog.
func (m *Model) ask(d *dialog) { m.dlg = d }

// choose closes the dialog with a choice, or none (-1).
func (m *Model) choose(i int) tea.Cmd {
	d := m.dlg
	m.dlg = nil
	if i < 0 {
		return nil
	}
	return d.answer(m, i)
}

func (m *Model) updateDialog(msg tea.KeyMsg) tea.Cmd {
	d := m.dlg
	if !d.since.IsZero() && time.Since(d.since) < dialogGrace {
		return nil
	}
	switch key := msg.String(); key {
	case "left", "h", "shift+tab", "up", "k":
		d.choice = max(0, d.choice-1)
	case "right", "l", "tab", "down", "j":
		d.choice = min(len(d.choices)-1, d.choice+1)
	case "enter", " ":
		return m.choose(d.choice)
	case "esc":
		return m.choose(d.cancel)
	default:
		if i, ok := d.keys[key]; ok {
			return m.choose(i)
		}
	}
	return nil
}

// dialogBox is a dialog laid out in the body.
type dialogBox struct {
	lines   []string
	x, y    int      // the top left corner in the body
	row     int      // the line of the buttons
	buttons [][2]int // the columns of each button, from the left edge
}

func (m *Model) layoutDialog() dialogBox {
	d := m.dlg
	var buttons []string
	var plain strings.Builder
	var cols [][2]int
	for i, c := range d.choices {
		if i > 0 {
			plain.WriteString("  ")
		}
		b := "[ " + c + " ]"
		at := ansi.StringWidth(plain.String())
		cols = append(cols, [2]int{at, at + ansi.StringWidth(b)})
		plain.WriteString(b)
		if i == d.choice {
			b = m.st.selected.Render(b)
		} else {
			b = m.st.name.Render(b)
		}
		buttons = append(buttons, b)
	}
	content := []string{""}
	for _, t := range d.text {
		content = append(content, m.st.name.Render(t))
	}
	content = append(content, "", strings.Join(buttons, "  "), "")
	buttonLine := len(content) - 2

	inner := 0
	for _, c := range content {
		inner = max(inner, ansi.StringWidth(c))
	}
	inner = min(inner+2, max(10, m.width-2)) // a space on each side
	h, v, corners := "─", "│", [4]string{"╭", "╮", "╰", "╯"}
	if m.opts.ASCII {
		h, v, corners = "-", "|", [4]string{"+", "+", "+", "+"}
	}
	top := corners[0] + h + fit(" "+d.title+" ", inner-1)
	top += strings.Repeat(h, max(0, inner+1-ansi.StringWidth(top))) + corners[1]
	box := dialogBox{lines: []string{m.st.section.Render(top)}, row: 1 + buttonLine}
	for _, c := range content {
		box.lines = append(box.lines, m.st.section.Render(v)+" "+pad(fit(c, inner-2), inner-2)+" "+m.st.section.Render(v))
	}
	box.lines = append(box.lines, m.st.section.Render(corners[2]+strings.Repeat(h, inner)+corners[3]))
	for _, c := range cols {
		box.buttons = append(box.buttons, [2]int{2 + c[0], 2 + c[1]})
	}
	box.x = max(0, (m.width-inner-2)/2)
	box.y = max(0, (m.bodyHeight()-len(box.lines))/2)
	return box
}

// clickDialog chooses the button at column x of body row y, if any.
func (m *Model) clickDialog(x, y int) tea.Cmd {
	box := m.layoutDialog()
	if y != box.y+box.row {
		return nil
	}
	for i, b := range box.buttons {
		if x >= box.x+b[0] && x < box.x+b[1] {
			return m.choose(i)
		}
	}
	return nil
}

// overlay draws the lines of a box over a block of lines, at column x of
// line y.
func overlay(block string, box []string, x, y int) string {
	lines := strings.Split(block, "\n")
	for i, b := range box {
		if y+i >= len(lines) {
			break
		}
		l := lines[y+i]
		left := ansi.Truncate(l, x, "")
		left += strings.Repeat(" ", max(0, x-ansi.StringWidth(left)))
		right := ansi.TruncateLeft(l, x+ansi.StringWidth(b), "")
		lines[y+i] = left + ansi.ResetStyle + b + ansi.ResetStyle + right
	}
	return strings.Join(lines, "\n")
}
