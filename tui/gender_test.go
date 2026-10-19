package tui

import (
	"slices"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/export"
	"github.com/efi/goged/gedcom"
)

// sexOfNames maps the names of the sample's people to their sex, leaving
// out names that people of different sex share.
func sexOfNames(doc *gedcom.Document) map[string]gedcom.Sex {
	out := map[string]gedcom.Sex{}
	shared := map[string]bool{}
	for _, ind := range doc.Individuals {
		name := ind.DisplayName()
		if sex, ok := out[name]; ok && sex != ind.Sex {
			shared[name] = true
		}
		out[name] = ind.Sex
	}
	for name := range shared {
		delete(out, name)
	}
	return out
}

// checkNameColors checks that every span holding just the name of a
// person is in the color of their sex, and reports how many there are.
func checkNameColors(t *testing.T, m *Model, ls []line) int {
	t.Helper()
	want := map[gedcom.Sex]lipgloss.TerminalColor{gedcom.SexFemale: m.st.female.GetForeground(), gedcom.SexMale: m.st.male.GetForeground()}
	sexes := sexOfNames(m.doc)
	n := 0
	for _, l := range ls {
		for _, sp := range l.spans {
			sex, ok := sexes[sp.text]
			if !ok {
				continue
			}
			got := sp.style.GetForeground()
			if c, colored := want[sex]; colored && got != c || !colored && (got == want[gedcom.SexFemale] || got == want[gedcom.SexMale]) {
				t.Errorf("%q (%s) in %q: color %v", sp.text, sex, l.plain(), got)
			}
			n++
		}
	}
	return n
}

func TestGenderColors(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	female, male := a.m.st.female.GetForeground(), a.m.st.male.GetForeground()
	heading := func() lipgloss.TerminalColor { return a.m.person.doc.lines[0].spans[0].style.GetForeground() }
	if heading() == male {
		t.Error("the names are colored before G")
	}

	a.press("G")
	a.contains("names are colored by sex: women light red, men light blue (G turns it off)")
	if heading() != male {
		t.Errorf("John Smith's name is %v", heading())
	}
	// Relatives, the partner in the family heading and the event titles,
	// relatives' events.
	if n := checkNameColors(t, &a.m, a.m.person.doc.lines); n < 8 {
		t.Errorf("%d names colored in the person view", n)
	}
	for _, l := range a.m.person.doc.lines {
		if l.heading && l.plain() == "Family with Ann Taylor" {
			if c := l.spans[0].style.GetForeground(); c != a.m.st.section.GetForeground() {
				t.Errorf("the heading is %v", c)
			}
			if c := l.spans[1].style.GetForeground(); c != female {
				t.Errorf("Ann Taylor in the heading is %v", c)
			}
		}
	}

	// A sex not given as female or male keeps the color.
	a.openPerson("I24")
	if c := heading(); c == female || c == male {
		t.Errorf("Infant Smith (sex U) is %v", c)
	}

	// The charts: the root keeps its underline, the marked person their
	// color.
	a.openPerson("I14")
	a.press("t")
	c := a.m.tree.chart
	a.m.reference = a.m.doc.Individual("I13") // Jane Doe
	for i, n := range c.Nodes {
		for _, line := range c.Lines {
			for _, seg := range line {
				if seg.Node != i {
					continue
				}
				style, _ := a.m.segmentStyle(c, seg, -1)
				switch {
				case n.Ind == a.m.reference:
					if style.GetForeground() != a.m.st.reference.GetForeground() {
						t.Errorf("the marked person is %v", style.GetForeground())
					}
				case n.Ind.Sex == gedcom.SexFemale && style.GetForeground() != female, n.Ind.Sex == gedcom.SexMale && style.GetForeground() != male:
					t.Errorf("%s in the chart is %v", n.Ind.DisplayName(), style.GetForeground())
				case i == 0 && !style.GetUnderline():
					t.Error("the root is not underlined")
				}
			}
		}
	}
	a.m.reference = nil

	// Both partners of a marriage in the events.
	marriage := a.m.doc.Family("F2").Marriage()
	if n := checkNameColors(t, &a.m, []line{a.m.eventLine(marriage, 100)}); n != 2 {
		t.Errorf("%d partners colored in %q", n, a.m.eventLine(marriage, 100).plain())
	}

	// In the search, G is typed; alt+G switches.
	a.press("/", "ctrl+u")
	a.typ("G")
	if v := a.m.search.input.Value(); v != "G" || !a.m.genders {
		t.Errorf("query %q, colored %v", v, a.m.genders)
	}
	a.press("alt+G")
	if a.m.genders {
		t.Error("alt+G left the colors on")
	}
	a.press("alt+G", "esc", "esc")

	// The statistics, and loading the file again.
	a.press("7")
	if n := checkNameColors(t, &a.m, a.m.stats.doc.lines); n < 1 {
		t.Error("no name colored in the statistics")
	}
	a.m.replaceDoc(loadSample(t))
	if !a.m.genders {
		t.Error("loading the file again turned the colors off")
	}
	a.press("G")
	a.contains("names are not colored by sex (G colors them)")
	for _, l := range a.m.stats.doc.lines {
		for _, sp := range l.spans {
			if c := sp.style.GetForeground(); c == female || c == male {
				t.Fatalf("%q is still colored", l.plain())
			}
		}
	}
}

// The colors change no text, at any width.
func TestGenderColorsKeepText(t *testing.T) {
	a := newApp(t, Options{})
	for _, width := range []int{100, 50} {
		for _, ind := range a.m.doc.Individuals {
			a.m.genders = false
			off := a.m.personLines(ind, width)
			a.m.genders = true
			on := a.m.personLines(ind, width)
			if len(on) != len(off) {
				t.Fatalf("%s: %d lines, %d without colors", ind.ID, len(on), len(off))
			}
			for i := range on {
				if on[i].plain() != off[i].plain() {
					t.Errorf("%s at %d: %q, without colors %q", ind.ID, width, on[i].plain(), off[i].plain())
				}
			}
		}
		var events []*gedcom.Event
		for _, ind := range a.m.doc.Individuals {
			events = append(events, ind.Events...)
		}
		for _, f := range a.m.doc.Families {
			events = append(events, f.Events...)
		}
		for _, ev := range events {
			a.m.genders = false
			off := a.m.eventLine(ev, width)
			a.m.genders = true
			on := a.m.eventLine(ev, width)
			if on.plain() != off.plain() || len(on.layout(width, true)) != len(off.layout(width, true)) {
				t.Errorf("at %d: %q, without colors %q", width, on.plain(), off.plain())
			}
		}
	}
}

// Exports are never colored.
func TestGenderColorsNotExported(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	a.press("G")
	female, male := a.m.st.female.GetForeground(), a.m.st.male.GetForeground()
	sel, _ := export.Select(a.m.doc, export.Options{})
	for _, b := range a.m.bookBlocks(sel, export.Options{}) {
		for _, l := range b.lines {
			for _, sp := range l.spans {
				if c := sp.style.GetForeground(); c == female || c == male {
					t.Fatalf("%q is colored in the export", l.plain())
				}
			}
		}
	}
	if !a.m.genders {
		t.Error("the export turned the colors off")
	}
}

func TestNameSpans(t *testing.T) {
	a := newApp(t, Options{})
	a.m.genders = true
	john, ann := a.m.doc.Individual("I3"), a.m.doc.Individual("I5")
	texts := func(spans []span) []string {
		var out []string
		for _, sp := range spans {
			out = append(out, sp.text)
		}
		return out
	}
	for _, tt := range []struct {
		text   string
		people []*gedcom.Individual
		want   []string
	}{
		{"John Smith & Ann Taylor", []*gedcom.Individual{ann, john}, []string{"John Smith", " & ", "Ann Taylor"}},
		{"Marriage with Ann Taylor", []*gedcom.Individual{nil, ann}, []string{"Marriage with ", "Ann Taylor"}},
		{"Ann Taylor's wife", []*gedcom.Individual{ann}, []string{"Ann Taylor", "'s wife"}},
		{"Birth", []*gedcom.Individual{ann}, []string{"Birth"}},
		{"", nil, []string{""}},
	} {
		if got := texts(a.m.nameSpans(tt.text, lipgloss.Style{}, tt.people...)); !slices.Equal(got, tt.want) {
			t.Errorf("%q: %q, want %q", tt.text, got, tt.want)
		}
	}
	a.m.genders = false
	if got := texts(a.m.nameSpans("John Smith & Ann Taylor", lipgloss.Style{}, john, ann)); len(got) != 1 {
		t.Errorf("without colors: %q", got)
	}

	// A column: the last name takes the room left, unless there is none.
	a.m.genders = true
	spans := asColumn(a.m.nameSpans("John Smith & Ann Taylor", lipgloss.Style{}, john, ann), 20)
	if len(spans) != 3 || spans[2].width != 7 {
		t.Errorf("column of 20: %+v", spans)
	}
	spans = asColumn(a.m.nameSpans("John Smith & Ann Taylor", lipgloss.Style{}, john, ann), 13)
	if len(spans) != 1 || spans[0].width != 13 || spans[0].style.GetForeground() != a.m.st.male.GetForeground() {
		t.Errorf("column of 13: %+v", spans)
	}
}
