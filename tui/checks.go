package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

// checksState holds the plausibility checks and the hints: their
// thresholds, which are switched on, their findings, and the rows shown:
// a heading per section, a row per check, followed (unless it is folded)
// by its summary, if any, and its findings.
type checksState struct {
	params    []int
	enabled   []bool
	folded    []bool
	results   [][]genealogy.Finding
	summaries []string
	rows      []checkRow
	cursor    int
	offset    int
	// placeStep is the place of a group of places that o opens next.
	placeStep int
}

// checkRow is a check, a finding (finding >= 0), a summary, a heading or
// the blank line before one.
type checkRow struct{ check, finding int }

const (
	rowCheck   = -1
	rowSummary = -2
	rowHeading = -3
	rowBlank   = -4
)

// selectable reports whether the cursor can rest on the row.
func (r checkRow) selectable() bool { return r.finding >= rowCheck }

// ensureChecks runs the checks the first time the view is shown, with the
// usual thresholds unless others are set.
func (m *Model) ensureChecks() {
	c := &m.checks
	if c.results != nil {
		return
	}
	n := len(genealogy.Checks)
	if c.params == nil {
		c.params, c.enabled, c.folded = make([]int, n), make([]bool, n), make([]bool, n)
		for i, ch := range genealogy.Checks {
			c.params[i], c.enabled[i] = ch.Param, true
		}
	}
	c.results, c.summaries = make([][]genealogy.Finding, n), make([]string, n)
	for i := range genealogy.Checks {
		m.runCheck(i)
	}
	m.checkRows()
}

func (m *Model) checkContext() genealogy.Context {
	return genealogy.Context{Marked: m.reference, Today: m.opts.Today}
}

// runCheck applies check i with its current threshold.
func (m *Model) runCheck(i int) {
	c := &m.checks
	c.results[i], c.summaries[i] = nil, ""
	if c.enabled[i] {
		ch, ctx := genealogy.Checks[i], m.checkContext()
		c.results[i] = ch.Run(m.doc, c.params[i], ctx)
		c.summaries[i] = ch.Summary(m.doc, c.params[i], ctx)
	}
}

// markChanged runs the checks again that depend on the marked person.
func (m *Model) markChanged() {
	if m.checks.results == nil {
		return
	}
	for i, ch := range genealogy.Checks {
		if ch.UsesMarked() {
			m.runCheck(i)
		}
	}
	m.checkRows()
}

// checkRows lists the rows, keeping the selection on the same check, and
// on the same finding if it is still there.
func (m *Model) checkRows() {
	c := &m.checks
	sel := checkRow{0, rowCheck}
	if c.cursor < len(c.rows) {
		sel = c.rows[c.cursor]
	}
	c.rows = c.rows[:0]
	c.cursor = 0
	for i, ch := range genealogy.Checks {
		if i > 0 && ch.Hint != genealogy.Checks[i-1].Hint {
			c.rows = append(c.rows, checkRow{i, rowBlank})
		}
		if i == 0 || ch.Hint != genealogy.Checks[i-1].Hint {
			c.rows = append(c.rows, checkRow{i, rowHeading})
		}
		if i == sel.check {
			c.cursor = len(c.rows)
		}
		c.rows = append(c.rows, checkRow{i, rowCheck})
		if c.folded[i] {
			continue
		}
		if c.summaries[i] != "" {
			c.rows = append(c.rows, checkRow{i, rowSummary})
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

// moveChecks moves the cursor by delta rows, past headings and summaries,
// and scrolls so that it shows, with the headings and summaries just
// above it.
func (m *Model) moveChecks(delta int) {
	c := &m.checks
	if len(c.rows) == 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	start := clamp(c.cursor+delta, 0, len(c.rows)-1)
	cur := start
	for cur >= 0 && cur < len(c.rows) && !c.rows[cur].selectable() {
		cur += dir
	}
	if cur < 0 || cur >= len(c.rows) {
		for cur = start; cur >= 0 && cur < len(c.rows) && !c.rows[cur].selectable(); cur -= dir {
		}
	}
	if cur != c.cursor {
		c.placeStep = 1
	}
	c.cursor = clamp(cur, 0, len(c.rows)-1)
	h := m.checksHeight()
	if c.cursor < c.offset {
		c.offset = c.cursor
	}
	if c.cursor >= c.offset+h {
		c.offset = c.cursor - h + 1
	}
	for c.offset > 0 && !c.rows[c.offset-1].selectable() && c.cursor-c.offset+1 < h {
		c.offset--
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

// openPlaceOf shows place k of a finding's group of places.
func (m *Model) openPlaceOf(f *genealogy.Finding, k int) {
	p := f.Places[k%len(f.Places)]
	m.showPlace(p.Full)
	m.setStatus(fmt.Sprintf("%s · place %d of %d in the group; o in the checks shows the next", p.Full, k%len(f.Places)+1, len(f.Places)))
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
		f := m.finding()
		switch {
		case f == nil:
			c.folded[i] = !c.folded[i]
			m.checkRows()
		case f.Person != nil:
			m.open(f.Person)
		case len(f.Places) > 0:
			m.openPlaceOf(f, 0)
			c.placeStep = 1
		}
	case "o":
		f := m.finding()
		switch {
		case f == nil:
		case f.Other != nil:
			m.open(f.Other)
		case len(f.Places) > 1:
			m.openPlaceOf(f, c.placeStep)
			c.placeStep++
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
		m.setStatus("all checks and hints are on, with the usual thresholds")
	default:
		return false, nil
	}
	return true, nil
}

// checkRowOf returns the row of check i.
func (m *Model) checkRowOf(i int) int {
	for r, row := range m.checks.rows {
		if row.check == i && row.finding == rowCheck {
			return r
		}
	}
	return 0
}

// sectionCounts counts the findings of the checks or the hints, and the
// people they concern.
func (m Model) sectionCounts(hints bool) string {
	findings := 0
	people := map[*gedcom.Individual]bool{}
	for i, rs := range m.checks.results {
		if genealogy.Checks[i].Hint != hints {
			continue
		}
		findings += len(rs)
		for _, f := range rs {
			if f.Person != nil {
				people[f.Person] = true
			}
		}
	}
	return fmt.Sprintf(" · %s in %s", plural(findings, "finding", "findings"), plural(len(people), "person", "people"))
}

func (m Model) viewChecks(h int) string {
	c := &m.checks
	head := m.st.section.Render("Checks and hints") + m.st.dim.Render(" · ←→ threshold · space on/off · enter folds or opens · o the other person or place")
	out := []string{fit(head, m.width), ""}

	countW := 5
	nameW := max(10, m.width-2-countW-1)
	for r := c.offset; r < len(c.rows) && len(out) < h; r++ {
		row := c.rows[r]
		ch := genealogy.Checks[row.check]
		switch {
		case row.finding == rowBlank:
			out = append(out, "")
			continue
		case row.finding == rowHeading:
			title := "Checks"
			if ch.Hint {
				title = "Hints"
			}
			out = append(out, fit(m.st.section.Render(title)+m.st.dim.Render(m.sectionCounts(ch.Hint)), m.width))
			continue
		case row.finding == rowSummary:
			out = append(out, fit(m.st.dim.Render("      "+c.summaries[row.check]), m.width))
			continue
		case row.finding >= 0:
			f := c.results[row.check][row.finding]
			text := "    " + f.Message
			if f.Person != nil {
				text = "    " + f.Person.DisplayName()
				if ls := f.Person.Lifespan(); ls != "" {
					text += " (" + ls + ")"
				}
				if r == c.cursor {
					text += "  " + f.Message
				}
			}
			switch {
			case r == c.cursor:
				out = append(out, fit(m.st.selected.Render("▸ "+text), m.width))
			case f.Person != nil:
				out = append(out, fit("  "+m.st.name.Render(text)+"  "+f.Message, m.width))
			default:
				out = append(out, fit("  "+m.st.name.Render(text), m.width))
			}
			continue
		}

		marker, count := "  ", fmt.Sprintf("%*d", countW, len(c.results[row.check]))
		switch {
		case !c.enabled[row.check]:
			count = fmt.Sprintf("%*s", countW, "off")
		case len(c.results[row.check]) == 0 && c.summaries[row.check] == "":
		case c.folded[row.check]:
			marker = "▹ "
		default:
			marker = "▾ "
		}
		name := ch.Name(c.params[row.check])
		if r == c.cursor {
			out = append(out, fit(m.st.selected.Render("▸ "+pad(marker+name, nameW)+" "+count), m.width))
			continue
		}
		var title string
		switch {
		case !c.enabled[row.check]:
			title = m.st.dim.Render(pad(marker+name, nameW))
		case ch.HasParam():
			// The threshold stands out in the title.
			before, after := splitThreshold(ch.Title)
			value := ch.Value(c.params[row.check])
			plain := marker + before + value + after
			title = m.st.name.Render(marker+before) + m.st.key.Render(value) + m.st.name.Render(after) +
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

// splitThreshold returns the title before and after its threshold.
func splitThreshold(title string) (before, after string) {
	for _, verb := range []string{"%d", "%s"} {
		if b, a, ok := strings.Cut(title, verb); ok {
			return b, a
		}
	}
	return title, ""
}
