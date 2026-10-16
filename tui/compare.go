package tui

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/diff"
	"github.com/efi/goged/gedcom"
)

// The comparison (M): two people side by side, as diff.People pairs what
// their records hold, in an overlay like the help. The marked person is
// on the left.

type compareState struct {
	a, b *gedcom.Individual
}

// compareSelected compares the selected person with the marked one.
func (m *Model) compareSelected() {
	sel, ref := m.selected(), m.reference
	switch {
	case ref == nil:
		m.setError("mark a person with m first: M compares the selected person with them")
	case sel == nil:
		m.setStatus("select a person to compare with " + ref.DisplayName())
	case sel == ref:
		m.setStatus(sel.DisplayName() + " is the marked person: select another one to compare with them")
	default:
		m.comparePeople(ref, sel)
	}
}

// comparePeople shows a and b side by side.
func (m *Model) comparePeople(a, b *gedcom.Individual) {
	m.compare = compareState{a, b}
	m.openOverlay("Compare")
}

// buildCompare lays out the comparison for the width of the screen: a
// line on each person, which opens them, then a row for every detail,
// wrapped within its columns and marked with ≠ where they differ.
func (m *Model) buildCompare() {
	a, b := m.compare.a, m.compare.b
	if a == nil || b == nil {
		m.help.set([]line{dimLine(m, "the people compared are not in the file any more")})
		return
	}
	rows := diff.People(a, b)
	differ, whatW := 0, 0
	for _, r := range rows {
		if !r.Same() {
			differ++
		}
		whatW = max(whatW, ansi.StringWidth(r.What))
	}
	whatW = min(whatW, labelWidth)
	width := max(40, m.width-2) // the overlay keeps two columns for the selection
	colW := max(10, (width-2-whatW-2-2)/2)
	gap := span{text: "  ", width: 2}

	ls := []line{
		textLine(span{text: "Comparison of two people", style: m.st.name}),
		dimLine(m, fmt.Sprintf("%s, %d the same · enter opens a person", plural(differ, "difference", "differences"), len(rows)-differ)),
		textLine(),
		m.relativeLine("Marked", a, "  "+a.ID),
		m.relativeLine("Compared", b, "  "+b.ID),
		textLine(),
		textLine(span{text: "", width: 2 + whatW}, gap, span{text: a.DisplayName(), style: m.st.label, width: colW}, gap,
			span{text: b.DisplayName(), style: m.st.label}),
	}
	for _, r := range rows {
		mark, style := "", m.st.dim
		if !r.Same() {
			mark, style = "≠", m.st.name.UnsetBold()
		}
		ls = append(ls, textLine(span{text: mark, style: m.st.key, width: 2}, span{text: r.What, style: m.st.label, width: whatW}, gap,
			span{text: r.Left, style: style, width: colW}, gap, span{text: r.Right, style: style}))
	}
	m.help.set(expand(ls, width))
}
