package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
)

// The gender colors (alt+G): while they are on, the names of women are light
// red and those of men light blue, wherever a person's name is shown on
// its own, in a title or in a list. The status line and dialogs keep
// their colors, and so does the marked person in the charts, whose color
// is what marks them there. Exports are never colored; a printed chart
// is, as it is a picture of the screen.

// toggleGenders switches the gender colors on or off.
func (m *Model) toggleGenders() {
	m.genders = !m.genders
	m.rebuildPerson()
	m.person.doc.ensureVisible(m.bodyHeight())
	if m.stats.computed {
		m.buildStats()
	}
	if m.showHelp {
		m.rebuildOverlay()
	}
	if m.genders {
		m.setStatus("names are colored by sex: women light red, men light blue (alt+G turns it off)")
	} else {
		m.setStatus("names are not colored by sex (alt+G colors them)")
	}
}

// genderColor returns the color of ind's sex while the gender colors are
// on; false if they are off or the sex is not given as female or male.
func (m *Model) genderColor(ind *gedcom.Individual) (lipgloss.TerminalColor, bool) {
	if !m.genders || ind == nil {
		return nil, false
	}
	switch ind.Sex {
	case gedcom.SexFemale:
		return m.st.female.GetForeground(), true
	case gedcom.SexMale:
		return m.st.male.GetForeground(), true
	}
	return nil, false
}

// personStyle returns base for the name of ind: in the color of their sex
// while the gender colors are on.
func (m *Model) personStyle(ind *gedcom.Individual, base lipgloss.Style) lipgloss.Style {
	if c, ok := m.genderColor(ind); ok {
		return base.Foreground(c)
	}
	return base
}

// nameSpans returns text in style, split so that the names of the people
// given are in personStyle.
func (m *Model) nameSpans(text string, style lipgloss.Style, people ...*gedcom.Individual) []span {
	var out []span
	for m.genders && text != "" {
		at, n := -1, 0
		var who *gedcom.Individual
		for _, p := range people {
			if p == nil {
				continue
			}
			name := p.DisplayName()
			if i := strings.Index(text, name); name != "" && i >= 0 && (at < 0 || i < at) {
				at, n, who = i, len(name), p
			}
		}
		if at < 0 {
			break
		}
		if at > 0 {
			out = append(out, span{text: text[:at], style: style})
		}
		out = append(out, span{text: text[at : at+n], style: m.personStyle(who, style)})
		text = text[at+n:]
	}
	if text != "" || len(out) == 0 {
		out = append(out, span{text: text, style: style})
	}
	return out
}

// renderNames renders text in style, the names of the people given in
// personStyle.
func (m *Model) renderNames(text string, style lipgloss.Style, people ...*gedcom.Individual) string {
	var b strings.Builder
	for _, sp := range m.nameSpans(text, style, people...) {
		b.WriteString(sp.style.Render(sp.text))
	}
	return b.String()
}

// asColumn makes spans one column width wide: the last span takes the
// room the others leave. If they leave it none, the spans become one, in
// the style of the first.
func asColumn(spans []span, width int) []span {
	used := 0
	for _, sp := range spans[:len(spans)-1] {
		used += ansi.StringWidth(sp.text)
	}
	if len(spans) > 1 && used < width-1 {
		spans[len(spans)-1].width = width - used
		return spans
	}
	var text strings.Builder
	for _, sp := range spans {
		text.WriteString(sp.text)
	}
	return []span{{text: text.String(), style: spans[0].style, width: width}}
}
