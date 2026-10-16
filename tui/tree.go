package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/gedcom"
	"github.com/mattn/go-runewidth"
)

type treeMode int

const (
	modePedigree treeMode = iota
	modeDescendants
	modeHourglass
	modeLifespans
	treeModes
)

const (
	minGenerations = 2
	// Both charts show each person's relatives only once, so they grow
	// with the number of people, not exponentially with the generations.
	maxPedigreeGenerations   = 20
	maxDescendantGenerations = 20
	// Lifespans count the generations each way, and show every relative.
	maxLifespanGenerations = 10
	fullLabels             = 1 << 16 // a label width limit that is never reached
)

// generationLimits returns the fewest and the most generations shown in
// the current chart.
func (t *treeState) generationLimits() (lo, hi int) {
	switch t.mode {
	case modeDescendants:
		return minGenerations, maxDescendantGenerations
	case modeLifespans:
		return 1, maxLifespanGenerations
	}
	return minGenerations, maxPedigreeGenerations
}

// generations returns the generations of the current chart. Lifespans
// have their own, as they count each way.
func (t *treeState) generations() *int {
	if t.mode == modeLifespans {
		return &t.lifeGens
	}
	return &t.gens
}

// lifespanReach says how far a lifespan chart reaches.
func lifespanReach(n int) string {
	switch n {
	case 1:
		return "parents to children"
	case 2:
		return "grandparents to grandchildren"
	case 3:
		return "great-grandparents to great-grandchildren"
	}
	return fmt.Sprintf("%d generations up and down", n)
}

type treeState struct {
	mode     treeMode
	gens     int
	lifeGens int // of the lifespans
	chart    *chart.Chart
	sel      int
	top      int // first visible chart line
	left     int // first visible display column
	// expanded holds where the pedigree of expandedFor shows the
	// ancestors of people who appear more than once (chart.Options).
	expanded    map[*gedcom.Individual]string
	expandedFor *gedcom.Individual
}

// rebuildTree renders the chart for the current person and selects them.
func (m *Model) rebuildTree() {
	t := &m.tree
	ind := m.current()
	if ind == nil {
		t.chart = nil
		return
	}
	opts := chart.Options{Generations: t.gens, MaxLabel: 36, ASCII: m.opts.ASCII}
	switch t.mode {
	case modeLifespans:
		// The chart fits the width, so it is drawn again on a resize.
		t.chart = chart.Lifespans(ind, chart.LifespanOptions{Generations: t.lifeGens, Width: m.width, ASCII: m.opts.ASCII})
	case modeDescendants:
		// Every person has a line of their own and the view scrolls
		// sideways, so labels are shown in full.
		opts.MaxLabel = fullLabels
		t.chart = chart.Descendants(ind, opts)
	default:
		// The choices are paths from the root, so a new root starts afresh.
		if t.expandedFor != ind {
			t.expanded, t.expandedFor = nil, ind
		}
		opts.Expanded = t.expanded
		if t.mode == modeHourglass {
			t.chart = chart.Hourglass(ind, opts)
		} else {
			t.chart = chart.Pedigree(ind, opts)
		}
	}
	t.sel, t.top, t.left = 0, 0, 0
	if t.mode == modeHourglass {
		// The root is in the middle of the hourglass.
		root := t.chart.Nodes[0]
		t.left = max(0, root.Col+root.Width/2-m.width/2)
		t.top = max(0, root.Line-m.treeHeight()/2)
	}
	m.ensureTreeVisible()
}

// hourglassStep returns the node to the left (dir < 0) or to the right of
// the selected one in an hourglass chart, where the descendants are on the
// left: arrows move as the chart is drawn.
func (m *Model) hourglassStep(dir int) int {
	t := &m.tree
	node := t.chart.Nodes[t.sel]
	away := func(left bool) int {
		for _, d := range node.Down {
			if t.chart.Nodes[d].Left == left {
				return d
			}
		}
		return t.sel
	}
	switch {
	case t.sel == 0:
		return away(dir < 0)
	case node.Left == (dir < 0):
		return away(node.Left)
	}
	return node.Up
}

func (m *Model) treeHeight() int { return max(1, m.bodyHeight()-1) }

// treeGap returns the blank lines between the header and the chart: one
// for the lifespans if there is room.
func (m *Model) treeGap() int {
	if c := m.tree.chart; m.tree.mode == modeLifespans && c != nil && len(c.Lines)+2 <= m.treeHeight() {
		return 1
	}
	return 0
}

// treeRows returns the screen rows for the lines of the chart that
// scroll, between those of its head and foot (and the line on the person
// selected in the lifespans), and the first and last of these lines.
func (m *Model) treeRows() (rows, first, end int) {
	c := m.tree.chart
	if c == nil {
		return m.treeHeight(), 0, 0
	}
	rows = m.treeHeight() - c.Head - c.Foot
	if m.tree.mode == modeLifespans {
		rows--
	}
	return max(1, rows), c.Head, len(c.Lines) - c.Foot
}

// ensureTreeVisible scrolls the chart so the selected node is on screen.
// For pedigree charts the root starts vertically centered.
func (m *Model) ensureTreeVisible() {
	t := &m.tree
	if t.chart == nil || len(t.chart.Nodes) == 0 {
		return
	}
	w := m.width
	h, first, end := m.treeRows()
	n := t.chart.Nodes[t.sel]
	if n.Line < t.top {
		t.top = n.Line
	}
	if n.Line >= t.top+h {
		t.top = n.Line - h + 1
	}
	t.top = clamp(t.top, first, max(first, end-h))
	if n.Col < t.left {
		t.left = max(0, n.Col-4)
	}
	if n.Col+n.Width > t.left+w {
		t.left = n.Col + n.Width - w + 2
		if n.Width > w {
			t.left = n.Col
		}
	}
	t.left = clamp(t.left, 0, max(0, t.chart.Width-w+2))
}

func (m *Model) selectTreeNode(i int) {
	t := &m.tree
	if t.chart == nil || i < 0 || i >= len(t.chart.Nodes) {
		return
	}
	t.sel = i
	m.ensureTreeVisible()
}

func (m *Model) updateTree(msg tea.KeyMsg) (bool, tea.Cmd) {
	t := &m.tree
	if t.chart == nil || len(t.chart.Nodes) == 0 {
		return false, nil
	}
	node := t.chart.Nodes[t.sel]
	switch msg.String() {
	case "up", "k":
		m.selectTreeNode(t.chart.Next(t.sel, -1))
	case "down", "j":
		m.selectTreeNode(t.chart.Next(t.sel, 1))
	case "left", "right", "l":
		dir := 1
		if msg.String() == "left" {
			dir = -1
		}
		switch {
		case t.mode == modeHourglass:
			m.selectTreeNode(m.hourglassStep(dir))
		case dir < 0:
			m.selectTreeNode(node.Up)
		case len(node.Down) > 0:
			m.selectTreeNode(node.Down[0])
		}
	case "pgup", "pgdown":
		dir := 1
		if msg.String() == "pgup" {
			dir = -1
		}
		target := node.Line + dir*m.treeHeight()
		sel := t.sel
		for {
			next := t.chart.Next(sel, dir)
			if next == sel || (dir > 0 && t.chart.Nodes[next].Line > target) || (dir < 0 && t.chart.Nodes[next].Line < target) {
				break
			}
			sel = next
		}
		if sel == t.sel {
			sel = t.chart.Next(sel, dir)
		}
		m.selectTreeNode(sel)
	case "home", "g":
		m.selectTreeNode(0)
	case "enter", "i", " ", "r":
		key := msg.String()
		switch {
		case node.Kind == chart.KindDuplicate:
			m.expandDuplicate()
		case key == "enter" || key == "i":
			m.open(node.Ind)
		case node.Ind == m.current():
			m.setStatus(node.Ind.DisplayName() + " is already the root")
		default:
			m.visit(node.Ind)
		}
	case "p":
		m.setTreeMode(modePedigree)
	case "d":
		m.setTreeMode(modeDescendants)
	case "h":
		m.setTreeMode(modeHourglass)
	case "s":
		m.setTreeMode(modeLifespans)
	case "v":
		m.setTreeMode((t.mode + 1) % treeModes)
	case "+", "=":
		m.setGenerations(*t.generations() + 1)
	case "-", "_":
		m.setGenerations(*t.generations() - 1)
	default:
		return false, nil
	}
	return true, nil
}

// expandDuplicate shows the ancestors of the person whose duplicate
// placeholder is selected at this place in the pedigree; the person's other
// occurrences get the placeholder instead. The selection moves to the
// first ancestor shown in its place.
func (m *Model) expandDuplicate() {
	t := &m.tree
	person := t.chart.Nodes[t.chart.Nodes[t.sel].Up]
	if t.expanded == nil {
		t.expanded = map[*gedcom.Individual]string{}
	}
	t.expanded[person.Ind] = person.Path
	m.rebuildTree()
	i := t.chart.FindPath(person.Path)
	if i < 0 {
		return
	}
	n := t.chart.Nodes[i]
	if len(n.Down) == 0 || t.chart.Nodes[n.Down[0]].Kind == chart.KindDuplicate {
		// The place is among the person's own ancestors (a loop in the
		// data), so the choice was dropped.
		m.selectTreeNode(i)
		m.setError("the ancestors of " + n.Ind.DisplayName() + " cannot be shown here")
		return
	}
	m.selectTreeNode(n.Down[0])
	m.setStatus("the ancestors of " + n.Ind.DisplayName() + " are shown here now and truncated elsewhere")
}

func (m *Model) setTreeMode(mode treeMode) {
	if m.tree.mode == mode {
		return
	}
	m.tree.mode = mode
	if _, limit := m.tree.generationLimits(); *m.tree.generations() > limit {
		*m.tree.generations() = limit
		m.setStatus(fmt.Sprintf("showing %d generations, the most for this chart", limit))
	}
	m.rebuildTree()
}

func (m *Model) setGenerations(n int) {
	t := &m.tree
	lo, hi := t.generationLimits()
	n = clamp(n, lo, hi)
	if n == *t.generations() {
		m.setStatus(fmt.Sprintf("generations are limited to %d–%d", lo, hi))
		return
	}
	*t.generations() = n
	if t.mode == modeLifespans {
		m.setStatus("showing " + lifespanReach(n))
	} else {
		m.setStatus(fmt.Sprintf("showing %d generations", n))
	}
	sel := t.chart.Nodes[t.sel]
	m.rebuildTree()
	// Keep the selection on the same person if they are still shown, in a
	// pedigree at the same place (or the person of a placeholder).
	i := -1
	if t.mode != modeDescendants && t.mode != modeLifespans && !sel.Left {
		i = t.chart.FindPath(sel.Path)
	}
	if i < 0 {
		i = m.tree.chart.Find(sel.Ind)
	}
	if i >= 0 {
		m.selectTreeNode(i)
	}
}

func (m Model) viewTree(h int) string {
	t := &m.tree
	if t.chart == nil || m.current() == nil {
		return m.st.dim.Render("  No person selected. Press / to search.")
	}
	kind := [treeModes]string{"Pedigree", "Descendants", "Hourglass", "Lifespans"}[t.mode]
	sel := t.chart.Nodes[t.sel]
	// The selected person comes last; if the line is too long, the root's
	// name and then the number of generations are left out so that the
	// selection stays readable.
	gens := fmt.Sprintf(" · %d generations", t.gens)
	if t.mode == modeLifespans {
		gens = " · " + lifespanReach(t.lifeGens)
	}
	selected := m.st.dim.Render(" · selected: ") + m.st.name.Render(chart.Label(sel.Ind))
	if sel.Kind == chart.KindDuplicate {
		selected = m.st.dim.Render(" · selected: ancestors of ") + m.st.name.Render(chart.Label(sel.Ind)) +
			m.st.dim.Render(", shown elsewhere; enter or space shows them here")
	}
	header := m.st.section.Render(kind+" of "+m.current().DisplayName()) + m.st.dim.Render(gens) + selected
	if ansi.StringWidth(header) > m.width {
		header = m.st.section.Render(kind) + m.st.dim.Render(gens) + selected
	}
	if ansi.StringWidth(header) > m.width {
		header = m.st.section.Render(kind) + selected
	}
	out := []string{fit(header, m.width)}
	if m.treeGap() > 0 {
		out = append(out, "")
	}
	rows, first, end := m.treeRows()
	for i := range first {
		out = append(out, m.renderTreeLine(i))
	}
	for i := t.top; i < end && i < t.top+rows; i++ {
		out = append(out, m.renderTreeLine(i))
	}
	// The line on the person selected in the lifespans follows the
	// blank line before the legend.
	about := t.mode == modeLifespans
	for i := end; i < len(t.chart.Lines); i++ {
		out = append(out, m.renderTreeLine(i))
		if about && len(t.chart.Lines[i]) == 0 {
			out = append(out, m.lifespanAbout())
			about = false
		}
	}
	if about {
		out = append(out, "", m.lifespanAbout())
	}
	return strings.Join(out[:min(len(out), h)], "\n")
}

// lifespanAbout describes the life of the person selected in the
// lifespans: the name, then the rest dimmed.
func (m Model) lifespanAbout() string {
	about := m.tree.chart.About(m.tree.sel)
	name, rest, _ := strings.Cut(about, " · ")
	return fit(m.st.name.Render(name)+m.st.dim.Render(" · "+rest), m.width)
}

// renderTreeLine draws the visible horizontal slice of a chart line.
func (m Model) renderTreeLine(i int) string {
	t := &m.tree
	left, right := t.left, t.left+m.width
	var b strings.Builder
	col := 0
	for _, seg := range t.chart.Lines[i] {
		w := runewidth.StringWidth(seg.Text)
		start, end := col, col+w
		col = end
		if end <= left || start >= right {
			continue
		}
		text := seg.Text
		if start < left || end > right {
			text = ansi.Cut(text, max(0, left-start), min(w, right-start))
		}
		switch {
		case seg.Node >= 0 && seg.Node == t.sel:
			b.WriteString(m.st.selected.Render(text))
		case seg.Class == chart.ClassText:
			b.WriteString(text)
		case seg.Class == chart.ClassBar:
			b.WriteString(m.st.bar.Render(text))
		case seg.Class == chart.ClassRootBar:
			b.WriteString(m.st.name.Render(text))
		case seg.Class == chart.ClassMarker:
			b.WriteString(m.st.mapMarker.Render(text))
		case seg.Node < 0 || seg.Class == chart.ClassDim:
			b.WriteString(m.st.dim.Render(text))
		case t.chart.Nodes[seg.Node].Kind == chart.KindDuplicate:
			b.WriteString(m.st.dim.Render(text))
		case t.chart.Nodes[seg.Node].Ind == m.reference:
			b.WriteString(m.st.reference.Render(text))
		case seg.Node == 0:
			b.WriteString(m.st.root.Render(text))
		default:
			b.WriteString(text)
		}
	}
	return b.String()
}
