package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/genealogy"
)

type statsState struct {
	doc      docView
	computed bool
}

const maxListedWarnings = 200

// ensureStats computes the statistics the first time they are needed.
func (m *Model) ensureStats() {
	if m.stats.computed {
		return
	}
	m.stats.computed = true
	s := genealogy.Compute(m.doc)
	var ls []line
	row := func(label, value string) line {
		return textLine(span{pad(label, 22), m.st.label}, span{value, m.st.name.UnsetBold()})
	}
	blank := textLine()

	ls = append(ls, m.sectionLine("File"))
	if m.opts.Title != "" {
		ls = append(ls, row("Name", m.opts.Title))
	}
	ls = append(ls, row("Encoding", string(m.doc.Encoding)))
	if m.doc.Version != "" {
		ls = append(ls, row("GEDCOM version", m.doc.Version))
	}
	if sw := m.doc.SourceSoftware(); sw != "" {
		ls = append(ls, row("Created by", sw))
	}

	ls = append(ls, blank, m.sectionLine("Records"))
	ls = append(ls,
		row("Individuals", fmt.Sprintf("%d  (%d male, %d female, %d other/unknown)", s.Individuals, s.Males, s.Females, s.OtherSex)),
		row("Families", fmt.Sprint(s.Families)),
		row("Events", fmt.Sprint(s.Events)),
		row("Places", fmt.Sprint(s.Places)),
		row("Sources", fmt.Sprint(s.Sources)),
	)

	ls = append(ls, blank, m.sectionLine("Time"))
	if s.EarliestYear != 0 {
		ls = append(ls, row("Years covered", fmt.Sprintf("%d–%d", s.EarliestYear, s.LatestYear)))
	}
	ls = append(ls, row("Generations", fmt.Sprint(s.Generations)))
	if s.Lifespans > 0 {
		ls = append(ls, row("Average lifespan", fmt.Sprintf("%.1f years (%d people with known birth and death)", s.AverageLifespan, s.Lifespans)))
	}
	if s.LongestLived != nil {
		l := row("Longest lived", fmt.Sprintf("%s, %s years", s.LongestLived.DisplayName(), s.LongestAge))
		l.target = s.LongestLived
		ls = append(ls, l)
	}

	if len(s.Surnames) > 0 {
		ls = append(ls, blank, m.sectionLine("Most common surnames"), dimLine(m, "press enter to list everybody with that surname"))
		for _, nc := range s.Surnames[:min(30, len(s.Surnames))] {
			l := textLine(span{pad(nc.Name, 22), m.st.name}, span{fmt.Sprint(nc.Count), m.st.dim})
			l.query = fmt.Sprintf("surname:%q", nc.Name)
			ls = append(ls, l)
		}
	}
	if len(s.GivenNames) > 0 {
		ls = append(ls, blank, m.sectionLine("Most common given names"))
		var parts []string
		for _, nc := range s.GivenNames[:min(15, len(s.GivenNames))] {
			parts = append(parts, fmt.Sprintf("%s (%d)", nc.Name, nc.Count))
		}
		for _, w := range wrap(strings.Join(parts, ", "), max(20, m.width-4)) {
			ls = append(ls, textLine(span{w, m.st.name.UnsetBold()}))
		}
	}

	ls = append(ls, blank)
	ls = append(ls, m.onThisDay(row)...)

	ls = append(ls, blank, m.sectionLine(fmt.Sprintf("Warnings (%d)", len(m.doc.Warnings))))
	if len(m.doc.Warnings) == 0 {
		ls = append(ls, dimLine(m, "none — the file looks consistent"))
	}
	for i, w := range m.doc.Warnings {
		if i == maxListedWarnings {
			ls = append(ls, dimLine(m, fmt.Sprintf("… and %d more", len(m.doc.Warnings)-maxListedWarnings)))
			break
		}
		ls = append(ls, dimLine(m, w.String()))
	}
	m.stats.doc.set(ls)
}

// onThisDay lists the events that happened on today's day and month in
// earlier years; each line links to the person (or the first partner).
func (m *Model) onThisDay(row func(label, value string) line) []line {
	today := m.opts.Today
	if today.IsZero() {
		today = time.Now()
	}
	day := today.Format("2 January")
	ls := []line{m.sectionLine("On this day (" + day + ")")}
	annivs := genealogy.OnThisDay(m.doc, today)
	if len(annivs) == 0 {
		return append(ls, dimLine(m, "no events recorded for "+day))
	}
	for _, a := range annivs {
		year := fmt.Sprint(a.Year)
		if a.Year <= 0 {
			year = fmt.Sprintf("%d BC", 1-a.Year)
		}
		switch a.YearsAgo {
		case 0:
			year += " · today"
		case 1:
			year += " · 1 year ago"
		default:
			year += fmt.Sprintf(" · %d years ago", a.YearsAgo)
		}
		var names []string
		people := a.Event.Principals()
		for _, p := range people {
			names = append(names, p.DisplayName())
		}
		l := row(year, a.Event.Label()+"  "+strings.Join(names, " & "))
		if place := a.Event.Place.String(); place != "" {
			l.spans = append(l.spans, span{"  " + place, m.st.dim})
		}
		if len(people) > 0 {
			l.target = people[0]
		}
		ls = append(ls, l)
	}
	return ls
}

func (m *Model) updateStats(msg tea.KeyMsg) (bool, tea.Cmd) {
	d := &m.stats.doc
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
		sel := d.selected()
		if sel == nil || !d.visible(d.selectedLine(), h) {
			return true, nil
		}
		if sel.target != nil {
			m.open(sel.target)
			return true, nil
		}
		m.switchTo(viewSearch)
		m.setQuery(sel.query)
	default:
		return false, nil
	}
	return true, nil
}

func (m Model) viewStats(h int) string {
	return m.stats.doc.render(m.width, h, m.st)
}
