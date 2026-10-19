package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

type statsState struct {
	doc      docView
	computed bool
	stats    genealogy.Stats
}

const maxListedWarnings = 200

// ensureStats computes the statistics the first time they are needed.
func (m *Model) ensureStats() {
	if m.stats.computed {
		return
	}
	m.stats.computed = true
	m.stats.stats = genealogy.Compute(m.doc)
	m.buildStats()
}

// buildStats lays out the statistics for the width of the screen.
func (m *Model) buildStats() {
	s := m.stats.stats
	var ls []line
	// row is a label and a value, which may name people.
	row := func(label, value string, people ...*gedcom.Individual) line {
		return textLine(append([]span{{text: pad(label, 22), style: m.st.label}}, m.nameSpans(value, m.st.name.UnsetBold(), people...)...)...)
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
		l := row("Longest lived", fmt.Sprintf("%s, %s years", s.LongestLived.DisplayName(), s.LongestAge), s.LongestLived)
		l.target = s.LongestLived
		ls = append(ls, l)
	}

	if len(s.Surnames) > 0 {
		ls = append(ls, blank, m.sectionLine("Most common surnames"), dimLine(m, "enter: the people with that surname, or the surname on the map"))
		for _, nc := range s.Surnames[:min(30, len(s.Surnames))] {
			l := textLine(span{text: pad(nc.Name, 22), style: m.st.name}, span{text: fmt.Sprint(nc.Count), style: m.st.dim})
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
			ls = append(ls, textLine(span{text: w, style: m.st.name.UnsetBold()}))
		}
	}

	ls = append(ls, blank)
	ls = append(ls, m.onThisDay(row)...)

	if s.AncestorPositions > 0 {
		ls = append(ls, blank, m.sectionLine("Pedigree collapse"),
			row("Repeated ancestors", fmt.Sprintf("%.1f%% of all ancestor positions · %s", s.PedigreeCollapse(), s.CollapsedPeople())),
			dimLine(m, "every place in someone's pedigree is a position; it repeats when an ancestor is there more than once, as with married cousins"))
	}
	ls = append(ls, m.statsCharts(s)...)

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

// askSurname asks what to show of a surname: the people with it (query
// finds them), or its places on the map.
func (m *Model) askSurname(name, query string) {
	m.ask(&dialog{
		title:   name,
		text:    []string{"What would you like to see of " + name + "?"},
		choices: []string{"People with that surname", "Surname on the map"},
		keys:    map[string]int{"p": 0, "m": 1},
		cancel:  -1,
		hints:   [][2]string{{"p", "people"}, {"m", "map"}, {"esc", "cancel"}},
		answer: func(m *Model, choice int) tea.Cmd {
			if choice == 0 {
				m.switchTo(viewSearch)
				m.setQuery(query)
			} else {
				m.showSurnameOnMap(name)
			}
			return nil
		},
	})
}

// birthPeriods are the widths of the ranges of birth years, with their
// names; the first that needs no more than maxBars bars is used.
var birthPeriods = []struct {
	years int
	name  string
}{{10, "decade"}, {20, "20 years"}, {25, "25 years"}, {50, "50 years"}, {100, "century"}, {500, "500 years"}, {1000, "millennium"}}

const maxBars = 25

// statsCharts draws the distributions as bar charts. A bar of births
// links to a search for the people born then.
func (m *Model) statsCharts(s genealogy.Stats) []line {
	var ls []line
	if len(s.BirthYears) > 0 {
		var buckets []genealogy.Bucket
		var name string
		for _, p := range birthPeriods {
			if buckets, name = s.BirthYears.Buckets(p.years, math.MaxInt), p.name; len(buckets) <= maxBars {
				break
			}
		}
		ls = append(ls, m.barChart("Births per "+name, buckets, func(b genealogy.Bucket) (string, string) {
			if b.Hi-b.Lo == 9 && b.Lo%10 == 0 && b.Lo >= 0 {
				return fmt.Sprintf("%ds", b.Lo), fmt.Sprintf("born:%d..%d", b.Lo, b.Hi)
			}
			return fmt.Sprintf("%d–%d", b.Lo, b.Hi), fmt.Sprintf("born:%d..%d", b.Lo, b.Hi)
		})...)
	}
	ages := func(b genealogy.Bucket) (string, string) {
		if b.Open {
			return fmt.Sprintf("%d+", b.Lo), ""
		}
		if b.Lo == b.Hi {
			return fmt.Sprint(b.Lo), ""
		}
		return fmt.Sprintf("%d–%d", b.Lo, b.Hi), ""
	}
	if len(s.DeathAges) > 0 {
		ls = append(ls, m.barChart("Age at death", s.DeathAges.Buckets(10, 100), ages)...)
	}
	if len(s.MarriageAges) > 0 {
		ls = append(ls, m.barChart("Age at marriage (each partner)", s.MarriageAges.Buckets(5, 50), ages)...)
	}
	if len(s.FamilySizes) > 0 {
		ls = append(ls, m.barChart("Children per family", s.FamilySizes.Buckets(1, 12), ages)...)
	}
	return ls
}

// barChart draws a heading and a bar for every bucket, as long as the
// screen allows for the largest. label names a bucket and gives the query
// its line links to, if any.
func (m *Model) barChart(title string, buckets []genealogy.Bucket, label func(genealogy.Bucket) (string, string)) []line {
	most, labelW := 0, 0
	for _, b := range buckets {
		most = max(most, b.Count)
		l, _ := label(b)
		labelW = max(labelW, ansi.StringWidth(l))
	}
	countW := len(fmt.Sprint(most))
	room := max(4, m.width-4-labelW-2-countW-1)
	ls := []line{textLine(), m.sectionLine(title)}
	for _, b := range buckets {
		l, query := label(b)
		row := textLine(
			span{text: l, style: m.st.label, width: labelW + 2},
			span{text: fmt.Sprintf("%*d", countW, b.Count), style: m.st.dim, width: countW + 1},
			span{text: bar(b.Count, most, room, m.opts.ASCII), style: m.st.bar})
		row.query, row.colsOnly = query, true
		if b.Count == 0 {
			row.query = ""
		}
		ls = append(ls, row)
	}
	return ls
}

// bar draws n of most as a line up to room columns long, in half columns;
// any count shows. Lines rather than blocks keep the charts light.
func bar(n, most, room int, ascii bool) string {
	if n == 0 || most == 0 {
		return ""
	}
	if ascii {
		return strings.Repeat("=", max(1, (n*room+most/2)/most))
	}
	halves := max(1, (n*room*2+most/2)/most)
	return strings.Repeat("━", halves/2) + []string{"", "╸"}[halves%2]
}

// onThisDay lists the events that happened on today's day and month in
// earlier years; each line links to the person (or the first partner).
func (m *Model) onThisDay(row func(label, value string, people ...*gedcom.Individual) line) []line {
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
		l := row(year, a.Event.Label()+"  "+strings.Join(names, " & "), people...)
		if place := a.Event.Place.String(); place != "" {
			l.spans = append(l.spans, span{text: "  " + place, style: m.st.dim})
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
		if name, ok := strings.CutPrefix(sel.query, "surname:"); ok {
			if unquoted, err := strconv.Unquote(name); err == nil {
				name = unquoted
			}
			m.askSurname(name, sel.query)
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
