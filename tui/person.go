package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

type personState struct {
	doc docView
	// hideRelatives removes relatives' events from the timeline.
	hideRelatives bool
}

const labelWidth = 14

func gendered(sex gedcom.Sex, male, female, neutral string) string {
	switch sex {
	case gedcom.SexMale:
		return male
	case gedcom.SexFemale:
		return female
	}
	return neutral
}

// relativeLine is a selectable line "Label   Name (lifespan)".
func (m *Model) relativeLine(label string, ind *gedcom.Individual, suffix string) line {
	spans := []span{{pad(label, labelWidth), m.st.label}}
	spans = append(spans, m.personSpans(ind)...)
	if suffix != "" {
		spans = append(spans, span{suffix, m.st.dim})
	}
	return line{spans: spans, target: ind}
}

func dimLine(m *Model, text string) line { return textLine(span{text, m.st.dim}) }

// pedigreeSuffix marks non-birth links, e.g. "  (adopted)".
func pedigreeSuffix(p gedcom.Pedigree) string {
	if p.IsBirth() {
		return ""
	}
	return "  (" + string(p) + ")"
}

// rebuildPerson regenerates the person view for the current person.
func (m *Model) rebuildPerson() {
	ind := m.current()
	if ind == nil {
		m.person.doc.set(nil)
		return
	}
	var ls []line
	blank := textLine()
	textW := max(20, m.width-4)

	// Heading.
	ls = append(ls, textLine(span{ind.DisplayName(), m.st.name}))
	facts := []string{ind.Sex.String(), ind.ID}
	if life := ind.Lifespan(); life != "" {
		facts = append(facts, life)
	}
	for _, ref := range ind.RefNumbers() {
		facts = append(facts, "ref. "+ref)
	}
	ls = append(ls, dimLine(m, strings.Join(facts, " · ")))
	var called []string
	if n := ind.Name(); n.CallName != "" {
		called = append(called, "call name "+n.CallName)
	}
	if n := ind.Name(); n.Nickname != "" {
		called = append(called, "called "+n.Nickname)
	}
	if len(called) > 0 {
		ls = append(ls, dimLine(m, strings.Join(called, " · ")))
	}
	for _, alt := range ind.Names[min(1, len(ind.Names)):] {
		text := alt.String()
		if alt.Type != "" {
			text += " (" + alt.Type + ")"
		}
		ls = append(ls, textLine(span{pad("Also known as", labelWidth), m.st.label}, span{text, m.st.name}))
	}
	if m.reference != nil {
		ls = append(ls, m.relationshipLine(ind))
	}

	// Parents.
	ls = append(ls, blank, m.sectionLine("Parents"))
	if len(ind.FamiliesAsChild()) == 0 {
		ls = append(ls, dimLine(m, "none recorded"))
	}
	for _, l := range ind.FamiliesAsChild() {
		if len(l.Family.Partners()) == 0 {
			ls = append(ls, dimLine(m, "unknown (family "+l.Family.ID+")"))
		}
	}
	for _, pl := range ind.ParentLinks() {
		ls = append(ls, m.relativeLine(gendered(pl.Parent.Sex, "Father", "Mother", "Parent"), pl.Parent, pedigreeSuffix(pl.Pedigree)))
	}

	// Siblings.
	sibs, halves := ind.Siblings(), ind.HalfSiblings()
	if len(sibs)+len(halves) > 0 {
		ls = append(ls, blank, m.sectionLine("Siblings"))
		for _, s := range sibs {
			label, suffix := gendered(s.Sex, "Brother", "Sister", "Sibling"), ""
			switch r := genealogy.Relate(ind, s); {
			case r.Kind == genealogy.KindBlood && r.Half:
				label = gendered(s.Sex, "Half-brother", "Half-sister", "Half-sibling")
			case r.Kind == genealogy.KindAdoptive:
				suffix = "  (" + r.Pedigree.Adjective() + ")"
			}
			ls = append(ls, m.relativeLine(label, s, suffix))
		}
		for _, s := range halves {
			ls = append(ls, m.relativeLine(gendered(s.Sex, "Half-brother", "Half-sister", "Half-sibling"), s, ""))
		}
	}

	// Families.
	for _, f := range ind.FamiliesAsSpouse() {
		partner := f.Partner(ind)
		title := "Family"
		if partner != nil {
			title += " with " + partner.DisplayName()
		}
		var facts []string
		if marr := f.Marriage(); marr != nil {
			fact := "married"
			if marr.Date.IsValid() {
				fact += " " + marr.Date.String()
			}
			if p := marr.Place.String(); p != "" {
				fact += ", " + p
			}
			facts = append(facts, fact)
		}
		if div := f.FirstEvent("DIV", "ANUL"); div != nil {
			fact := strings.ToLower(div.Label())
			if div.Date.IsValid() {
				fact += " " + div.Date.String()
			}
			facts = append(facts, fact)
		}
		heading := []span{{title, m.st.section}}
		if len(facts) > 0 {
			heading = append(heading, span{"  " + strings.Join(facts, " · "), m.st.dim})
		}
		ls = append(ls, blank, textLine(heading...))
		if partner != nil {
			ls = append(ls, m.relativeLine(gendered(partner.Sex, "Husband", "Wife", "Spouse"), partner, ""))
		}
		if len(f.Children) == 0 {
			ls = append(ls, dimLine(m, "no children recorded"))
		}
		for _, c := range f.Children {
			l, _ := c.ChildLink(f)
			ls = append(ls, m.relativeLine(gendered(c.Sex, "Son", "Daughter", "Child"), c, pedigreeSuffix(l.Of(ind))))
		}
		for _, note := range f.Notes() {
			for _, w := range wrap(note, textW-2) {
				ls = append(ls, dimLine(m, "  "+w))
			}
		}
	}

	// Associations and records that may describe the same person.
	if assoc := m.associationLines(ind, textW); len(assoc) > 0 {
		ls = append(ls, blank, m.sectionLine("Associations"))
		ls = append(ls, assoc...)
	}

	// Events.
	timeline := genealogy.Timeline(ind, genealogy.TimelineOptions{Relatives: !m.person.hideRelatives})
	ls = append(ls, blank, m.sectionLine("Events"))
	if len(timeline) == 0 {
		ls = append(ls, dimLine(m, "none recorded"))
	}
	for _, e := range timeline {
		ls = append(ls, m.timelineLine(e))
		if e.Own() {
			var extra []string
			for _, f := range e.Event.Facts() {
				extra = append(extra, f.Label+": "+f.Value)
			}
			extra = append(extra, e.Event.Notes...)
			for _, text := range extra {
				for _, w := range wrap(text, textW-22) {
					ls = append(ls, dimLine(m, strings.Repeat(" ", 20)+w))
				}
			}
		}
	}

	// Notes and sources.
	if notes := ind.Notes(); len(notes) > 0 {
		ls = append(ls, blank, m.sectionLine("Notes"))
		for i, note := range notes {
			if i > 0 {
				ls = append(ls, blank)
			}
			for _, w := range wrap(note, textW) {
				ls = append(ls, textLine(span{w, m.st.dim.UnsetForeground()}))
			}
		}
	}
	cits := ind.Citations()
	for _, f := range ind.FamiliesAsSpouse() {
		cits = append(cits, f.Citations()...)
	}
	if len(cits) > 0 {
		ls = append(ls, blank, m.sectionLine("Sources"))
		seen := map[string]bool{}
		for _, c := range cits {
			text := c.Text
			if c.Page != "" {
				text += " — " + c.Page
			}
			if seen[text+"\x00"+c.Quote] {
				continue
			}
			seen[text+"\x00"+c.Quote] = true
			ls = append(ls, textLine(span{text, m.st.dim.UnsetForeground()}))
			if c.Quote != "" {
				for _, w := range wrap("“"+c.Quote+"”", textW-2) {
					ls = append(ls, dimLine(m, "  "+w))
				}
			}
		}
	}
	if ch := ind.Changed(); ch != "" {
		ls = append(ls, blank, dimLine(m, "Record last changed "+ch))
	}
	m.person.doc.set(ls)
}

// associationLines lists the records that may describe the same person
// (ALIA), the people associated with ind (ASSO) and the people ind is
// associated with.
func (m *Model) associationLines(ind *gedcom.Individual, textW int) []line {
	var ls []line
	for _, a := range ind.Aliases() {
		ls = append(ls, m.relativeLine("Same person?", a, "  (alias record)"))
	}
	// labelFor fits the relation into the label column: "Godparent",
	// "Witness" for "Witness of marriage" at a marriage, or "Associate" with
	// the relation in the context.
	labelFor := func(a *gedcom.Association, suffix string) (string, string) {
		full := a.Label()
		if len(full+suffix) < labelWidth {
			return full + suffix, ""
		}
		first, rest, _ := strings.Cut(full, " ")
		if strings.HasPrefix(rest, "of ") && len(first+suffix) < labelWidth {
			if a.Event != nil && strings.Contains(strings.ToLower(a.Event.Label()), strings.ToLower(strings.TrimPrefix(rest, "of "))) {
				return first + suffix, ""
			}
			return first + suffix, strings.ToLower(full)
		}
		if r := strings.TrimSpace(a.Relation); r != "" && !strings.Contains(r, "_") {
			full = r // as written, e.g. "langjähriger Weggefährte"
		}
		return "Associate" + suffix, full
	}
	context := func(a *gedcom.Association, extra string) string {
		var parts []string
		if extra != "" {
			parts = append(parts, extra)
		}
		if e := a.Event; e != nil {
			at := "at " + strings.ToLower(e.Label())
			if e.Date.IsValid() {
				at += " " + e.Date.String()
			}
			parts = append(parts, at)
		}
		if len(parts) == 0 {
			return ""
		}
		return "  (" + strings.Join(parts, " · ") + ")"
	}
	notes := func(a *gedcom.Association) {
		for _, note := range a.Notes {
			for _, w := range wrap(note, textW-labelWidth-2) {
				ls = append(ls, dimLine(m, strings.Repeat(" ", labelWidth)+w))
			}
		}
	}
	for _, a := range ind.Associations() {
		label, extra := labelFor(a, "")
		ls = append(ls, m.relativeLine(label, a.To, context(a, extra)))
		notes(a)
	}
	for _, a := range ind.AssociatedBy() {
		label, extra := labelFor(a, " of")
		ps := a.Principals()
		if len(ps) == 0 {
			continue
		}
		spans := []span{{pad(label, labelWidth), m.st.label}}
		if a.Family != nil {
			spans = append(spans, span{a.Family.Title(), m.st.name})
		} else {
			spans = append(spans, m.personSpans(ps[0])...)
		}
		if c := context(a, extra); c != "" {
			spans = append(spans, span{c, m.st.dim})
		}
		ls = append(ls, line{spans: spans, target: ps[0]})
		notes(a)
	}
	return ls
}

// relationshipLine describes how ind relates to the reference person.
func (m *Model) relationshipLine(ind *gedcom.Individual) line {
	label := span{pad("Relationship", labelWidth), m.st.label}
	ref := m.reference
	if ref == ind {
		return textLine(label, span{"★ this is the reference person", m.st.reference})
	}
	r := genealogy.Relate(ref, ind)
	if r.Kind == genealogy.KindNone {
		return textLine(label, span{"no relationship to " + ref.DisplayName() + " found", m.st.dim})
	}
	spans := []span{label, {ref.DisplayName() + "'s " + r.Description, m.st.reference}}
	if (r.Kind == genealogy.KindBlood || r.Kind == genealogy.KindAdoptive) && r.UpA > 0 && r.UpB > 0 && len(r.CommonAncestors) > 0 {
		var names []string
		for _, a := range r.CommonAncestors {
			names = append(names, a.DisplayName())
		}
		word := "common ancestor"
		if len(names) > 1 {
			word += "s"
		}
		spans = append(spans, span{"  (" + word + ": " + strings.Join(names, " & ") + ")", m.st.dim})
	}
	return textLine(spans...)
}

// timelineLine formats one timeline entry.
func (m *Model) timelineLine(e genealogy.TimelineEntry) line {
	ev := e.Event
	date, dateRest := "—", ""
	if !ev.Date.IsZero() {
		date, dateRest = ev.Date.Fit(19)
	}
	title := e.Title()
	var rest []string
	if d := ev.Detail(); d != "" {
		rest = append(rest, d)
	}
	if dateRest != "" {
		rest = append(rest, dateRest) // what did not fit the column
	}
	if p := ev.Place.String(); p != "" {
		rest = append(rest, p)
	}
	if e.HasAge {
		rest = append(rest, "age "+e.Age.String())
	}
	if ev.Restriction != "" {
		rest = append(rest, ev.Restriction)
	}
	tail := ""
	if len(rest) > 0 {
		tail = "  " + strings.Join(rest, " · ")
	}
	if !e.Own() {
		return line{spans: []span{{pad(date, 20), m.st.dim}, {title + tail, m.st.dim}}, target: e.Relative}
	}
	return textLine(span{pad(date, 20), m.st.label}, span{title, m.st.name.UnsetBold()}, span{tail, m.st.dim})
}

func (m *Model) updatePerson(msg tea.KeyMsg) (bool, tea.Cmd) {
	d := &m.person.doc
	h := m.bodyHeight()
	switch msg.String() {
	case "up", "k":
		d.move(-1, h)
	case "down", "j":
		d.move(1, h)
	case "pgup", "ctrl+u":
		d.page(-1, h)
	case "pgdown", "ctrl+d", " ":
		d.page(1, h)
	case "home", "g":
		d.home(h)
	case "end", "G":
		d.end(h)
	case "enter":
		if sel := d.selected(); sel != nil && d.visible(d.selectedLine(), h) {
			m.visit(sel.target)
		}
	case "left", "h":
		m.back()
	case "right", "l":
		m.forward()
	case "t", "p":
		m.tree.mode = modePedigree
		m.rebuildTree()
		m.switchTo(viewTree)
	case "d":
		m.tree.mode = modeDescendants
		m.rebuildTree()
		m.switchTo(viewTree)
	case "c":
		m.person.hideRelatives = !m.person.hideRelatives
		m.rebuildPerson()
		if m.person.hideRelatives {
			m.setStatus("hiding relatives' events")
		} else {
			m.setStatus("showing relatives' events")
		}
	default:
		return false, nil
	}
	return true, nil
}

func (m Model) viewPerson(h int) string {
	if m.current() == nil {
		return m.st.dim.Render("  No person selected. Press / to search.")
	}
	return m.person.doc.render(m.width, h, m.st)
}
