package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/chart"
	"github.com/mattn/go-runewidth"
)

type treeMode int

const (
	modePedigree treeMode = iota
	modeDescendants
)

const (
	minGenerations = 2
	// Pedigree charts grow exponentially with each generation, descendant
	// charts only with the actual number of descendants.
	maxPedigreeGenerations   = 12
	maxDescendantGenerations = 20
	fullLabels               = 1 << 16 // a label width limit that is never reached
)

// maxGenerations is the largest number of generations shown in the
// current tree mode.
func (t *treeState) maxGenerations() int {
	if t.mode == modeDescendants {
		return maxDescendantGenerations
	}
	return maxPedigreeGenerations
}

type treeState struct {
	mode  treeMode
	gens  int
	chart *chart.Chart
	sel   int
	top   int // first visible chart line
	left  int // first visible display column
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
	if t.mode == modePedigree {
		t.chart = chart.Pedigree(ind, opts)
	} else {
		// Every person has a line of their own and the view scrolls
		// sideways, so labels are shown in full.
		opts.MaxLabel = fullLabels
		t.chart = chart.Descendants(ind, opts)
	}
	t.sel, t.top, t.left = 0, 0, 0
	m.ensureTreeVisible()
}

func (m *Model) treeHeight() int { return max(1, m.bodyHeight()-1) }

// ensureTreeVisible scrolls the chart so the selected node is on screen.
// For pedigree charts the root starts vertically centered.
func (m *Model) ensureTreeVisible() {
	t := &m.tree
	if t.chart == nil || len(t.chart.Nodes) == 0 {
		return
	}
	h, w := m.treeHeight(), m.width
	n := t.chart.Nodes[t.sel]
	if n.Line < t.top {
		t.top = n.Line
	}
	if n.Line >= t.top+h {
		t.top = n.Line - h + 1
	}
	t.top = clamp(t.top, 0, max(0, len(t.chart.Lines)-h))
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
	case "left", "h":
		m.selectTreeNode(node.Up)
	case "right", "l":
		if len(node.Down) > 0 {
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
	case "enter", "i":
		m.open(node.Ind)
	case " ", "r":
		if node.Ind == m.current() {
			m.setStatus(node.Ind.DisplayName() + " is already the root")
		} else {
			m.visit(node.Ind)
		}
	case "p":
		m.setTreeMode(modePedigree)
	case "d":
		m.setTreeMode(modeDescendants)
	case "v":
		m.setTreeMode(1 - t.mode)
	case "+", "=":
		m.setGenerations(t.gens + 1)
	case "-", "_":
		m.setGenerations(t.gens - 1)
	case "m":
		m.mark(node.Ind)
	default:
		return false, nil
	}
	return true, nil
}

func (m *Model) setTreeMode(mode treeMode) {
	if m.tree.mode == mode {
		return
	}
	m.tree.mode = mode
	if limit := m.tree.maxGenerations(); m.tree.gens > limit {
		m.tree.gens = limit
		m.setStatus(fmt.Sprintf("showing %d generations, the most for this chart", limit))
	}
	m.rebuildTree()
}

func (m *Model) setGenerations(n int) {
	n = clamp(n, minGenerations, m.tree.maxGenerations())
	if n == m.tree.gens {
		m.setStatus(fmt.Sprintf("generations are limited to %d–%d", minGenerations, m.tree.maxGenerations()))
		return
	}
	m.tree.gens = n
	m.setStatus(fmt.Sprintf("showing %d generations", n))
	sel := m.tree.chart.Nodes[m.tree.sel].Ind
	m.rebuildTree()
	// Keep the selection on the same person if they are still shown.
	if i := m.tree.chart.Find(sel); i >= 0 {
		m.selectTreeNode(i)
	}
}

func (m Model) viewTree(h int) string {
	t := &m.tree
	if t.chart == nil || m.current() == nil {
		return m.st.dim.Render("  No person selected. Press / to search.")
	}
	kind := "Pedigree"
	if t.mode == modeDescendants {
		kind = "Descendants"
	}
	sel := t.chart.Nodes[t.sel]
	// The selected person comes last; if the line is too long, the root's
	// name and then the number of generations are left out so that the
	// selection stays readable.
	gens := fmt.Sprintf(" · %d generations", t.gens)
	selected := m.st.dim.Render(" · selected: ") + m.st.name.Render(chart.Label(sel.Ind))
	header := m.st.section.Render(kind+" of "+m.current().DisplayName()) + m.st.dim.Render(gens) + selected
	if ansi.StringWidth(header) > m.width {
		header = m.st.section.Render(kind) + m.st.dim.Render(gens) + selected
	}
	if ansi.StringWidth(header) > m.width {
		header = m.st.section.Render(kind) + selected
	}
	out := []string{fit(header, m.width)}
	for i := t.top; i < len(t.chart.Lines) && len(out) < h; i++ {
		out = append(out, m.renderTreeLine(i))
	}
	return strings.Join(out, "\n")
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
		case seg.Node < 0:
			b.WriteString(m.st.dim.Render(text))
		case seg.Node == t.sel:
			b.WriteString(m.st.selected.Render(text))
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
