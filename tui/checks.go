package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

// checksState holds the plausibility checks: their thresholds, which are
// switched on, their findings, and the rows shown: a row per check,
// followed by its findings unless it is folded.
type checksState struct {
	params  []int
	enabled []bool
	folded  []bool
	results [][]genealogy.Finding
	rows    []checkRow
	cursor  int
	offset  int
}

// checkRow is a check (finding -1) or one of its findings.
type checkRow struct{ check, finding int }

// ensureChecks runs the checks the first time the view is shown.
func (m *Model) ensureChecks() {
	c := &m.checks
	if c.params != nil {
		return
	}
	n := len(genealogy.Checks)
	c.params, c.enabled, c.folded, c.results = make([]int, n), make([]bool, n), make([]bool, n), make([][]genealogy.Finding, n)
	for i, ch := range genealogy.Checks {
		c.params[i], c.enabled[i] = ch.Param, true
		m.runCheck(i)
	}
	m.checkRows()
}

// runCheck applies check i with its current threshold.
func (m *Model) runCheck(i int) {
	c := &m.checks
	c.results[i] = nil
	if c.enabled[i] {
		c.results[i] = genealogy.Checks[i].Run(m.doc, c.params[i])
	}
}

// checkRows lists the rows, keeping the selection on the same check, and
// on the same finding if it is still there.
func (m *Model) checkRows() {
	c := &m.checks
	sel := checkRow{0, -1}
	if c.cursor < len(c.rows) {
		sel = c.rows[c.cursor]
	}
	c.rows = c.rows[:0]
	c.cursor = 0
	for i := range genealogy.Checks {
		if i == sel.check {
			c.cursor = len(c.rows)
		}
		c.rows = append(c.rows, checkRow{i, -1})
		if c.folded[i] {
			continue
		}
		for j := range c.results[i] {
			if i == sel.check && j == sel.finding {
				c.cursor = len(c.rows)
			}
			c.rows = append(c.rows, checkRow{i, j})
		}
	}
	m.moveChecks(0)
}

func (m *Model) checksHeight() int { return max(1, m.bodyHeight()-2) }

func (m *Model) moveChecks(delta int) {
	c := &m.checks
	c.cursor = clamp(c.cursor+delta, 0, len(c.rows)-1)
	h := m.checksHeight()
	if c.cursor < c.offset {
		c.offset = c.cursor
	}
	if c.cursor >= c.offset+h {
		c.offset = c.cursor - h + 1
	}
	c.offset = clamp(c.offset, 0, max(0, len(c.rows)-h))
}

// finding returns the selected finding, or nil on a check's row.
func (m *Model) finding() *genealogy.Finding {
	c := &m.checks
	if c.cursor >= len(c.rows) {
		return nil
	}
	r := c.rows[c.cursor]
	if r.finding < 0 {
		return nil
	}
	return &c.results[r.check][r.finding]
}

// setThreshold moves the threshold of the selected check by delta and
// applies the check again.
func (m *Model) setThreshold(delta int) {
	c := &m.checks
	i := c.rows[c.cursor].check
	ch := genealogy.Checks[i]
	if !ch.HasParam() {
		m.setStatus("this check has no threshold")
		return
	}
	c.params[i] = clamp(c.params[i]+delta, ch.Min, ch.Max)
	m.runCheck(i)
	m.checkRows()
	m.setStatus(fmt.Sprintf("%s: %s", ch.Name(c.params[i]), plural(len(c.results[i]), "finding", "findings")))
}

func (m *Model) updateChecks(msg tea.KeyMsg) (bool, tea.Cmd) {
	c := &m.checks
	if len(c.rows) == 0 {
		return false, nil
	}
	i := c.rows[c.cursor].check
	switch msg.String() {
	case "up", "k":
		m.moveChecks(-1)
	case "down", "j":
		m.moveChecks(1)
	case "pgup", "ctrl+u":
		m.moveChecks(-m.checksHeight())
	case "pgdown", "ctrl+d":
		m.moveChecks(m.checksHeight())
	case "home", "g":
		m.moveChecks(-len(c.rows))
	case "end", "G":
		m.moveChecks(len(c.rows))
	case "left", "h":
		m.setThreshold(-1)
	case "right", "l":
		m.setThreshold(1)
	case " ":
		c.enabled[i] = !c.enabled[i]
		m.runCheck(i)
		c.cursor = m.checkRowOf(i)
		m.checkRows()
	case "enter":
		if f := m.finding(); f != nil {
			m.open(f.Person)
			break
		}
		c.folded[i] = !c.folded[i]
		m.checkRows()
	case "o":
		if f := m.finding(); f != nil && f.Other != nil {
			m.open(f.Other)
		}
	case "-", "+", "=":
		for j := range c.folded {
			c.folded[j] = msg.String() == "-"
		}
		c.cursor = m.checkRowOf(i)
		m.checkRows()
	case "r":
		for j, ch := range genealogy.Checks {
			if c.params[j] != ch.Param || !c.enabled[j] {
				c.params[j], c.enabled[j] = ch.Param, true
				m.runCheck(j)
			}
		}
		m.checkRows()
		m.setStatus("all checks are on, with the usual thresholds")
	default:
		return false, nil
	}
	return true, nil
}

// checkRowOf returns the row of check i.
func (m *Model) checkRowOf(i int) int {
	for r, row := range m.checks.rows {
		if row.check == i && row.finding < 0 {
			return r
		}
	}
	return 0
}

func (m Model) viewChecks(h int) string {
	c := &m.checks
	findings := 0
	people := map[*gedcom.Individual]bool{}
	for _, rs := range c.results {
		findings += len(rs)
		for _, f := range rs {
			people[f.Person] = true
		}
	}
	summary := fmt.Sprintf(" · %s in %s · ←→ threshold · space on/off · enter folds or opens",
		plural(findings, "finding", "findings"), plural(len(people), "person", "people"))
	out := []string{fit(m.st.section.Render("Checks")+m.st.dim.Render(summary), m.width), ""}

	countW := 5
	for r := c.offset; r < len(c.rows) && len(out) < h; r++ {
		row := c.rows[r]
		ch := genealogy.Checks[row.check]
		if row.finding >= 0 {
			f := c.results[row.check][row.finding]
			text := "    " + f.Person.DisplayName()
			if ls := f.Person.Lifespan(); ls != "" {
				text += " (" + ls + ")"
			}
			if r == c.cursor {
				out = append(out, fit(m.st.selected.Render("▸ "+text+"  "+f.Message), m.width))
				continue
			}
			out = append(out, fit("  "+m.st.name.Render(text)+"  "+f.Message, m.width))
			continue
		}

		marker, count := "  ", fmt.Sprintf("%*d", countW, len(c.results[row.check]))
		switch {
		case !c.enabled[row.check]:
			count = fmt.Sprintf("%*s", countW, "off")
		case len(c.results[row.check]) > 0 && c.folded[row.check]:
			marker = "▹ "
		case len(c.results[row.check]) > 0:
			marker = "▾ "
		}
		// The threshold stands out in the title.
		name := ch.Title
		param := ""
		if ch.HasParam() {
			before, after, _ := strings.Cut(ch.Title, "%d")
			name, param = before, fmt.Sprint(c.params[row.check])+after
		}
		nameW := max(10, m.width-2-countW-1)
		if r == c.cursor {
			title := pad(marker+ch.Name(c.params[row.check]), nameW)
			out = append(out, fit(m.st.selected.Render("▸ "+title+" "+count), m.width))
			continue
		}
		var title string
		switch {
		case !c.enabled[row.check]:
			title = m.st.dim.Render(pad(marker+ch.Name(c.params[row.check]), nameW))
		case param != "":
			num, rest, _ := strings.Cut(param, " ")
			if rest != "" {
				rest = " " + rest
			}
			plain := marker + name + num + rest
			title = m.st.name.Render(marker+name) + m.st.key.Render(num) + m.st.name.Render(rest) +
				strings.Repeat(" ", max(0, nameW-len([]rune(plain))))
		default:
			title = m.st.name.Render(pad(marker+name, nameW))
		}
		style := m.st.dim
		if len(c.results[row.check]) > 0 {
			style = m.st.name
		}
		out = append(out, fit("  "+title+" "+style.Render(count), m.width))
	}
	return strings.Join(out, "\n")
}
