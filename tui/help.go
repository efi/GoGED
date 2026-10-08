package tui

import "strings"

// helpSections lists key bindings and the query syntax. Each entry is a
// pair of key (or example) and description; a single string is a heading.
var helpSections = [][]string{
	{"Global"},
	{"/", "search for people"},
	{"tab, shift+tab", "next / previous view"},
	{"1 … 6", "search, person, tree, events, places, stats (alt+1 … alt+6 also work while typing)"},
	{"b, backspace, [", "back to the previous person"},
	{"]", "forward in history"},
	{"i", "show the current person"},
	{"m", "mark the current person as reference (relationships are shown relative to them); again to clear"},
	{"?", "this help"},
	{"q, ctrl+c", "quit"},
	{""},
	{"Search"},
	{"type", "search as you type (see syntax below)"},
	{"↑ ↓ pgup pgdown", "select a result"},
	{"enter", "open the selected person"},
	{"esc", "clear the query, or return to the previous view"},
	{""},
	{"Person"},
	{"↑ ↓  j k", "move between relatives (parents, siblings, spouses, children, relatives' events)"},
	{"enter", "go to the selected relative"},
	{"← →  h l", "back / forward in history"},
	{"pgup pgdown g G", "scroll"},
	{"t / d", "pedigree / descendant tree of this person"},
	{"c", "show or hide relatives' events in the timeline"},
	{""},
	{"Tree"},
	{"↑ ↓", "previous / next person on screen"},
	{"← →", "towards / away from the root person"},
	{"enter", "open the selected person"},
	{"space, r", "make the selected person the root"},
	{"p / d / v", "pedigree / descendants / toggle"},
	{"+ / -", "more / fewer generations"},
	{"m", "mark the selected person as reference"},
	{""},
	{"Events"},
	{"↑ ↓ pgup pgdown", "select an event"},
	{"enter", "open the person (or first partner) of the event"},
	{"f", "edit the filter; enter to finish, esc to clear"},
	{"a", "toggle between important and all events"},
	{"x", "clear the filter"},
	{""},
	{"Places"},
	{"↑ ↓ pgup pgdown", "select a place; places are grouped by jurisdiction (country, county, town …)"},
	{"← →  h l", "collapse / expand a place, or go to the enclosing / first contained place"},
	{"- / +", "collapse / expand all places"},
	{"enter", "list the events at the place and all places within it; enter again opens the person, esc returns"},
	{"f", "filter places by name; enter to finish, esc to clear"},
	{""},
	{"Search syntax"},
	{"smith", "people with a name word starting with smith (diacritics are ignored)"},
	{"john smith", "all terms must match"},
	{`"van der berg"`, "a phrase"},
	{"~smyth", "names that sound alike (Soundex)"},
	{"given:john  surname:smith", "match one part of the name (also first:, last:, name:)"},
	{"born:1850  died:1900", "year of birth / death (baptism and burial count too)"},
	{"born:1840..1860", "year ranges; also <1900, >=1850, ~1850 (±5), 1850s"},
	{"born:leeds", "birth place (text instead of a year)"},
	{"place:york", "any event place"},
	{"year:1881", "any event in that year"},
	{"alive:1900", "possibly alive in that year"},
	{"sex:f", "m, f, u (unknown) or x"},
	{"id:I12", "a record identifier (typing I12 alone also works)"},
	{"occupation:weaver", "occupation contains"},
	{"note:mill  source:census", "notes / cited sources contain"},
	{"any:text", "any value in the record"},
	{"has:parents", "birth death parents father mother spouse children siblings notes sources media occupation"},
	{"tag:_MILT", "the record contains a tag; tag:BIRT.PLAC=leeds checks a tag path and value"},
	{"-term", "exclude matches, e.g. smith -given:john  or  -has:parents"},
	{"1850", "a bare year matches the year of birth or death"},
	{""},
	{"Event filter"},
	{"type:marr,div", "event types by tag or name (birth, death, marriage, census …)"},
	{"1850..1870", "events in a year range (or year:1850)"},
	{"place:leeds  name:smith", "place / people involved"},
	{"-type:birth", "exclude; plain words match type, names, place and details"},
}

func (m *Model) buildHelp() {
	var ls []line
	keyW := 0
	for _, s := range helpSections {
		if len(s) == 2 {
			keyW = max(keyW, len([]rune(s[0])))
		}
	}
	keyW = min(keyW+2, 28)
	descW := max(20, m.width-keyW-4)
	ls = append(ls, textLine(span{"goged — a terminal browser for GEDCOM family trees", m.st.name}), textLine())
	for _, s := range helpSections {
		switch {
		case len(s) == 1 && s[0] == "":
			ls = append(ls, textLine())
		case len(s) == 1:
			ls = append(ls, m.sectionLine(s[0]))
		default:
			for i, d := range wrap(s[1], descW) {
				k := ""
				if i == 0 {
					k = s[0]
				}
				ls = append(ls, textLine(span{pad(k, keyW), m.st.key}, span{d, m.st.name.UnsetBold()}))
			}
		}
	}
	m.help.set(ls)
}

// helpText returns the help as plain text (used by tests and the CLI).
func helpText() string {
	var b strings.Builder
	for _, s := range helpSections {
		b.WriteString(strings.Join(s, "  "))
		b.WriteByte('\n')
	}
	return b.String()
}
