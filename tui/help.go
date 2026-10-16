package tui

import (
	"strings"

	"github.com/efi/goged/fonts"
	"github.com/efi/goged/licenses"
	"github.com/efi/goged/worldmap"
)

// helpSections lists key bindings and the query syntax. Each entry is a
// pair of key (or example) and description; a single string is a heading.
var helpSections = [][]string{
	{"Global"},
	{"/", "search for people"},
	{"tab, shift+tab", "next / previous view"},
	{"1 … 9", "search, person, charts, events, places, map, stats, checks, export (alt+1 … alt+9 also work while typing)"},
	{"b, backspace, [", "back to the previous person"},
	{"]", "forward in history"},
	{"i", "show the current person"},
	{"m", "mark the selected person as reference (relationships are shown relative to them), else the current one; again to clear"},
	{"M", "compare the selected person with the marked one, side by side, the differences marked with ≠; enter on either opens them"},
	{"W", "wrap long lines in the person view instead of cutting them; again to cut"},
	{"y / n / N", "when another program has changed the file: load it again, not this time, never this session"},
	{"?", "this help"},
	{"L", "licenses of the map data and the software goged is built with"},
	{"D", "what changed when the file was last loaded again: header, people, families and other records; enter opens a person"},
	{"q, ctrl+c", "quit"},
	{""},
	{"Mouse"},
	{"click", "select a row, a relative or a person in a chart; click it again to do what enter does"},
	{"click a tab", "switch views; a click on the marked person (★) at the top opens them"},
	{"wheel", "scroll; on the map, zoom in or out around the pointer"},
	{"drag", "pan the map"},
	{"▾ ▹  ◂ ▸", "collapse or expand a place; change an export setting"},
	{"back, forward", "the mouse's side buttons go back and forward in history"},
	{"shift+drag", "select text in most terminals; or start goged with -no-mouse"},
	{""},
	{"Search"},
	{"type", "search as you type (see syntax below)"},
	{"↑ ↓ pgup pgdown", "select a result"},
	{"enter", "open the selected person"},
	{"esc", "clear the query, or return to the previous view"},
	{"alt+↑ / alt+↓", "earlier / later queries of this session; past the newest, a new empty line"},
	{"alt+m / alt+M", "mark the selected person as reference / compare them with the marked one"},
	{""},
	{"Person"},
	{"↑ ↓  j k", "move between relatives (parents, siblings, spouses, children, relatives' events)"},
	{"enter", "go to the selected relative"},
	{"← →  l", "back / forward in history"},
	{"pgup pgdown g G", "scroll"},
	{"t / d / h / s", "pedigree / descendant / hourglass chart / lifespans of this person"},
	{"c", "show or hide relatives' events in the timeline"},
	{""},
	{"Charts"},
	{"↑ ↓", "previous / next person; in a pedigree, of the same generation"},
	{"← →", "towards / away from the root person; in the hourglass, left and right as drawn: descendants are on the left"},
	{"enter", "open the selected person"},
	{"space, r", "make the selected person the root"},
	{"", "on a \"truncated as duplicate\" placeholder: show the ancestors there instead"},
	{"p / d / h / s / v", "pedigree / descendants / hourglass (descendants left, ancestors right) / lifespans / the next of these"},
	{"+ / -", "more / fewer generations; in the lifespans one more each way: grandparents, aunts and uncles, grandchildren"},
	{"P", "print the whole chart to a PDF file, as the screen would show it if it were large enough: A4, or larger as the chart needs; asks for the file name"},
	{"lifespans", "the lives of the parents, siblings, partners and children as lines on a time axis, coloured while the person lived; ⚭ marriages, ╍ approximate dates, ┄ birth or death not recorded; on the right how long they lived at the same time (for partners: were married)"},
	{""},
	{"Events"},
	{"↑ ↓ pgup pgdown", "select an event"},
	{"enter", "open the person (or first partner) of the event"},
	{"f", "edit the filter; enter to finish, esc to clear, alt+↑ / alt+↓ for earlier / later filters"},
	{"a", "toggle between important and all events"},
	{"x", "clear the filter"},
	{""},
	{"Places"},
	{"↑ ↓ pgup pgdown", "select a place; places are grouped by jurisdiction (country, county, town …)"},
	{"← →  h l", "collapse / expand a place, or go to the enclosing / first contained place"},
	{"- / +", "collapse / expand all places"},
	{"enter", "list the events at the place and all places within it; enter again opens the person, esc returns"},
	{"f", "filter places by name; enter to finish, esc to clear, alt+↑ / alt+↓ for earlier / later filters"},
	{"M", "show the selected place on the map"},
	{""},
	{"Map"},
	{"arrows  h j k l", "pan the map"},
	{"+ / -", "zoom in / out"},
	{"n / N", "next / previous place with coordinates (most events first)"},
	{"enter", "list the events at the selected place"},
	{"0 / w / c", "fit all places / show the whole map, 80°N to 60°S, with the equator in yellow / center on the selected place"},
	{"v", "the next projection: Equirectangular (latitudes evenly spaced, lands wider towards the poles, as the labels are), Equal Earth (areas true), Web Mercator (shapes true, as web maps)"},
	{"s", "the surname view: the places of the people born with a surname, with their events; again for all places"},
	{"PgUp PgDn", "in the surname view: the previous / next surname (most people first; or click one)"},
	{"e", "in the surname view: all events, births, deaths or other events"},
	{"coordinates", "are read from PLAC.MAP.LATI and LONG, e.g. LATI N50.781464, LONG E10.786089"},
	{""},
	{"Checks"},
	{"↑ ↓", "select a check or a hint, or one of its findings"},
	{"← →", "lower / raise the threshold of the check; the findings change at once"},
	{"space", "switch the check off or on"},
	{"enter", "fold or unfold the check; on a finding, open the person, or the first place of a group"},
	{"o", "open the other person of a finding (the child, partner or possible duplicate), or the next place of a group"},
	{"M", "compare a possible duplicate with the other record side by side, or the places of a group in a table (enter opens one); other findings: the person with the marked one"},
	{"- / +", "fold / unfold all checks"},
	{"r", "switch all checks on with their usual thresholds"},
	{"dates", "imprecise dates only count where the problem is certain: \"before 1850\" may be any time before, \"about\" up to two years off"},
	{"hints", "gaps rather than contradictions: unknown parents in the marked person's ancestry (with how complete each generation is), missing dates and places of birth, deaths, events, marriage dates and sources"},
	{"place names", "groups of places within the same place whose names may be one: ← → choose almost identical, similar or somewhat resembling"},
	{""},
	{"Export"},
	{"↑ ↓", "choose a setting"},
	{"← →", "change it: the format, and all people or those connected to the marked person (m)"},
	{"enter", "edit the file name, or export; an existing file is replaced after enter again"},
	{"GEXF", "people are nodes with sex, birth and death; parents point to their children, partners are linked both ways"},
	{"PDF", "contents, a page per person and the places with their events, as in the views, with links between the pages; nobody is marked"},
	{"HTML", "the same as one web page, with links between the sections"},
	{""},
	{"Search syntax"},
	{"smith", "people with a name word starting with smith (diacritics are ignored)"},
	{"john smith", "all terms must match"},
	{"smith | jones", "either (also OR); terms next to each other bind first: smith born:1850 | jones"},
	{"(smith | jones) born:<1850", "brackets group; -( … ) excludes a group"},
	{"place:leeds|york", "a | right after a value adds another value of the field"},
	{`"van der berg"`, "a phrase"},
	{"~smyth", "names that sound alike (Soundex and Cologne phonetics)"},
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
	{"ancestor:I20", "ancestors of a person, along birth links (the person view shows the ID)"},
	{"descendant:I1", "descendants of a person, along birth links"},
	{"ancestor:I20=2", "only these generations: 2 grandparents, ..3 up to great-grandparents, 0.. the person too"},
	{"-term", "exclude matches, e.g. smith -given:john  or  -has:parents"},
	{"1850", "a bare year matches the year of birth or death"},
	{""},
	{"Event filter"},
	{"type:marr,div", "event types by tag or name (birth, death, marriage, census …)"},
	{"1850..1870", "events in a year range (or year:1850)"},
	{"place:leeds  name:smith", "place / people involved"},
	{"-type:birth", "exclude; plain words match type, names, place and details"},
	{"type:birth (place:leeds | york)", "| or OR and brackets work as in the search"},
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
	ls = append(ls, textLine(span{text: "goged — a terminal browser for GEDCOM family trees", style: m.st.name}), textLine())
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
				ls = append(ls, textLine(span{text: pad(k, keyW), style: m.st.key}, span{text: d, style: m.st.name.UnsetBold()}))
			}
		}
	}
	m.help.set(ls)
}

// buildLicenses shows the licenses of the map data, of the font of the PDF
// export and of the third-party software compiled into goged.
func (m *Model) buildLicenses() {
	var ls []line
	ls = append(ls, m.sectionLine("Map data"))
	for _, l := range strings.Split(worldmap.License, "\n") {
		ls = append(ls, textLine(span{text: l, style: m.st.name.UnsetBold()}))
	}
	ls = append(ls, textLine(), m.sectionLine("Font"))
	for _, l := range strings.Split(strings.TrimRight(fonts.License, "\n"), "\n") {
		ls = append(ls, textLine(span{text: l, style: m.st.dim}))
	}
	ls = append(ls, textLine(), m.sectionLine("Third-party software"))
	for _, l := range strings.Split(strings.TrimRight(licenses.ThirdParty, "\n"), "\n") {
		if strings.HasPrefix(l, "== ") {
			ls = append(ls, textLine(span{text: strings.TrimPrefix(l, "== "), style: m.st.name}))
			continue
		}
		ls = append(ls, textLine(span{text: l, style: m.st.dim}))
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
