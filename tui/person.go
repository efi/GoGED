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
	ls = append(ls, dimLine(m, strings.Join(facts, " · ")))
	n := ind.Name()
	if n.Prefix != "" || n.Nickname != "" {
		var extra []string
		if n.Prefix != "" {
			extra = append(extra, "title "+n.Prefix)
		}
		if n.Nickname != "" {
			extra = append(extra, "called "+n.Nickname)
		}
		ls = append(ls, dimLine(m, strings.Join(extra, " · ")))
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
		suffix := ""
		if !l.IsBirth() {
			suffix = "  (" + strings.ToLower(l.Pedigree) + ")"
		}
		partners := l.Family.Partners()
		if len(partners) == 0 {
			ls = append(ls, dimLine(m, "unknown (family "+l.Family.ID+")"))
		}
		for _, p := range partners {
			ls = append(ls, m.relativeLine(gendered(p.Sex, "Father", "Mother", "Parent"), p, suffix))
		}
	}

	// Siblings.
	sibs, halves := ind.Siblings(), ind.HalfSiblings()
	if len(sibs)+len(halves) > 0 {
		ls = append(ls, blank, m.sectionLine("Siblings"))
		for _, s := range sibs {
			ls = append(ls, m.relativeLine(gendered(s.Sex, "Brother", "Sister", "Sibling"), s, ""))
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
			suffix := ""
			for _, l := range c.FamiliesAsChild() {
				if l.Family == f && !l.IsBirth() {
					suffix = "  (" + strings.ToLower(l.Pedigree) + ")"
				}
			}
			ls = append(ls, m.relativeLine(gendered(c.Sex, "Son", "Daughter", "Child"), c, suffix))
		}
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
			for _, note := range e.Event.Notes {
				for _, w := range wrap(note, textW-22) {
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
	if cits := ind.Citations(); len(cits) > 0 {
		ls = append(ls, blank, m.sectionLine("Sources"))
		seen := map[string]bool{}
		for _, c := range cits {
			text := c.Text
			if c.Page != "" {
				text += " — " + c.Page
			}
			if seen[text] {
				continue
			}
			seen[text] = true
			ls = append(ls, textLine(span{text, m.st.dim.UnsetForeground()}))
		}
	}
	m.person.doc.set(ls)
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
	if r.Kind == genealogy.KindBlood && r.UpA > 0 && r.UpB > 0 && len(r.CommonAncestors) > 0 {
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
	date := "—"
	switch {
	case ev.Date.IsValid():
		date = ev.Date.String()
	case !ev.Date.IsZero():
		date = strings.TrimSpace(ev.Date.Raw)
	}
	title := e.Title()
	if e.Own() && e.Spouse != nil {
		title += " with " + e.Spouse.DisplayName()
	}
	var rest []string
	if d := ev.Detail(); d != "" {
		rest = append(rest, d)
	}
	if p := ev.Place.String(); p != "" {
		rest = append(rest, p)
	}
	if e.HasAge {
		rest = append(rest, "age "+e.Age.String())
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
