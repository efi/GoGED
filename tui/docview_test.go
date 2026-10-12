package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/gedcom"
)

// docOf builds a document of n lines where the lines listed in links are
// selectable.
func docOf(n int, links ...int) *docView {
	isLink := map[int]bool{}
	for _, l := range links {
		isLink[l] = true
	}
	target := &gedcom.Individual{ID: "X"}
	var ls []line
	for i := range n {
		l := textLine(span{text: "line " + string(rune('A'+i%26))})
		if isLink[i] {
			l.target = target
		}
		ls = append(ls, l)
	}
	d := &docView{}
	d.set(ls)
	return d
}

func TestDocViewMove(t *testing.T) {
	d := docOf(30, 2, 5, 20)
	const h = 10
	if d.selectedLine() != 2 {
		t.Fatalf("first link = %d", d.selectedLine())
	}
	d.move(1, h)
	if d.selectedLine() != 5 || d.offset != 0 {
		t.Errorf("after down: line %d offset %d", d.selectedLine(), d.offset)
	}
	d.move(1, h)
	if d.selectedLine() != 20 || d.offset != 11 {
		t.Errorf("jump to far link: line %d offset %d", d.selectedLine(), d.offset)
	}
	// Past the last link the view scrolls to reveal the remaining lines.
	for range 15 {
		d.move(1, h)
	}
	if d.selectedLine() != 20 || d.offset != 20 {
		t.Errorf("scroll past last link: line %d offset %d", d.selectedLine(), d.offset)
	}
	if d.visible(20, h) != true || d.visible(19, h) {
		t.Error("visible")
	}
	d.move(-1, h)
	if d.selectedLine() != 5 || d.offset != 5 {
		t.Errorf("back up: line %d offset %d", d.selectedLine(), d.offset)
	}
	d.move(-1, h)
	if d.selectedLine() != 2 || d.offset != 0 {
		t.Errorf("first link shows the top of the document: line %d offset %d", d.selectedLine(), d.offset)
	}
	d.move(-1, h)
	if d.selectedLine() != 2 || d.offset != 0 {
		t.Error("stays at the top")
	}
}

func TestDocViewPageHomeEnd(t *testing.T) {
	d := docOf(50, 1, 3, 25, 40)
	const h = 10
	d.page(1, h)
	if d.offset != 9 {
		t.Errorf("offset after pgdown = %d", d.offset)
	}
	if d.selectedLine() != 1 {
		// No link is visible in lines 9-18, so the selection stays.
		t.Errorf("selection = %d", d.selectedLine())
	}
	d.page(1, h)
	if d.offset != 18 || d.selectedLine() != 25 {
		t.Errorf("pgdown: offset %d line %d", d.offset, d.selectedLine())
	}
	d.page(-1, h)
	if d.offset != 9 || d.selectedLine() != 25 && !d.visible(d.selectedLine(), h) {
		t.Errorf("pgup: offset %d line %d", d.offset, d.selectedLine())
	}
	d.end(h)
	if d.selectedLine() != 40 || d.offset != 40 {
		t.Errorf("end: line %d offset %d", d.selectedLine(), d.offset)
	}
	d.home(h)
	if d.selectedLine() != 1 || d.offset != 0 {
		t.Errorf("home: line %d offset %d", d.selectedLine(), d.offset)
	}
	d.reset()
	if d.cursor != 0 || d.offset != 0 {
		t.Error("reset")
	}
}

func TestDocViewWithoutLinks(t *testing.T) {
	d := docOf(30)
	const h = 10
	if d.selected() != nil || d.selectedLine() != -1 {
		t.Error("no selection")
	}
	d.move(5, h)
	if d.offset != 5 {
		t.Errorf("move scrolls: %d", d.offset)
	}
	d.page(1, h)
	if d.offset != 14 {
		t.Errorf("page scrolls: %d", d.offset)
	}
	d.move(100, h)
	if d.offset != 20 {
		t.Errorf("scroll clamps at the end: %d", d.offset)
	}
	d.end(h)
	d.ensureVisible(h)
	if d.offset != 20 {
		t.Errorf("end: %d", d.offset)
	}
	d.ensureVisible(0)
	d.set(nil)
	if d.offset != 0 || len(d.links) != 0 {
		t.Error("set(nil)")
	}
}

func TestDocViewRender(t *testing.T) {
	d := docOf(5, 1)
	st := defaultStyles()
	out := d.render(20, 7, st)
	lines := strings.Split(out, "\n")
	if len(lines) != 7 {
		t.Fatalf("render produced %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[1], "▸ line B") || !strings.HasPrefix(lines[0], "  line A") {
		t.Errorf("render = %q", lines)
	}
	if lines[6] != "" {
		t.Error("padding lines should be empty")
	}
	if narrow := d.render(4, 2, st); lipgloss.Width(strings.Split(narrow, "\n")[0]) > 4 {
		t.Errorf("lines must be truncated: %q", narrow)
	}
	if !d.lines[1].isLink() || d.lines[0].isLink() || d.lines[1].plain() != "line B" {
		t.Error("line helpers")
	}
	if l := (line{query: "x"}); !l.isLink() {
		t.Error("query lines are links")
	}
}

// rowTexts returns the rows of a layout as plain text.
func rowTexts(rows [][]span) []string {
	var out []string
	for _, r := range rows {
		var b strings.Builder
		for _, sp := range r {
			b.WriteString(sp.text)
		}
		out = append(out, b.String())
	}
	return out
}

// colOf returns the display column at which sub begins in s, or -1.
func colOf(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(s[:i])
}

func TestLayoutCut(t *testing.T) {
	// Columns are padded or cut to their width; the rest runs on, for the
	// screen to cut.
	l := textLine(span{text: "Witness", width: 10}, span{text: "Franz-Xaver Gabler"}, span{text: "  (at civil marriage 12 Sep 1968)"})
	if got := rowTexts(l.layout(20, false)); len(got) != 1 || got[0] != "Witness   Franz-Xaver Gabler  (at civil marriage 12 Sep 1968)" {
		t.Errorf("cut = %q", got)
	}
	if l.plain() != "Witness   Franz-Xaver Gabler  (at civil marriage 12 Sep 1968)" {
		t.Errorf("plain = %q", l.plain())
	}
	l = textLine(span{text: "Religious marriage", width: 10}, span{text: "|"})
	if got := l.plain(); got != "Religious…|" {
		t.Errorf("cut column = %q", got)
	}
}

func TestLayoutWrap(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true)
	// The text after a column continues where it began; hyphens may end a
	// row.
	l := textLine(span{text: "12 Sep 1968", width: 12}, span{text: "Civil marriage", style: bold}, span{text: "  Köln, Nordrhein-Westfalen, Deutschland"})
	rows := l.layout(40, true)
	want := []string{
		"12 Sep 1968 Civil marriage  Köln,",
		"            Nordrhein-Westfalen,",
		"            Deutschland",
	}
	if got := rowTexts(rows); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrapped =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if rows[0][2].text != "Civil marriage" || !rows[0][2].style.GetBold() {
		t.Errorf("styles are kept: %+v", rows[0])
	}

	// Every column continues in its own place.
	l = textLine(span{text: "4 Jul 1958", width: 12}, span{text: " "}, span{text: "Civil marriage", width: 10}, span{text: " "},
		span{text: "Karl Müller Junior & Rosina Thalhuber", width: 14}, span{text: " "}, span{text: "Sydney, Australien"})
	got := rowTexts(l.layout(60, true))
	if len(got) != 4 || colOf(got[0], "Sydney, Australien") != 39 || colOf(got[1], "marriage") != 13 ||
		colOf(got[1], "Junior &") != 24 || colOf(got[3], "Thalhuber") != 24 {
		t.Errorf("columns =\n%s", strings.Join(got, "\n"))
	}

	// Leading spaces are the indentation of further rows.
	l = textLine(span{text: "    note: a few words that wrap"})
	if got := rowTexts(l.layout(20, true)); strings.Join(got, "|") != "    note: a few|    words that wrap" {
		t.Errorf("indented = %q", got)
	}
}

func TestFlow(t *testing.T) {
	for _, tt := range []struct {
		text  string
		width int
		want  string
	}{
		{"Danzig-Westpreussen, Deutsches Reich", 19, "Danzig-|Westpreussen,|Deutsches Reich"},
		{"https://gov.genealogy.net/item/show/BROOWOJO93FH", 20, "https://gov.genealog|y.net/item/show/BROO|WOJO93FH"},
		{"a - b", 3, "a -|b"}, // a dash between spaces is no hyphen
		{"Mu\u0308ller Mu\u0308ller", 6, "Mu\u0308ller|Mu\u0308ller"},
		{"\u7530\u4E2D\u592A\u90CE", 5, "\u7530\u4E2D|\u592A\u90CE"},
		{"", 5, ""},
	} {
		var got []string
		for _, row := range flow([]span{{text: tt.text}}, tt.width, tt.width, 0) {
			var b strings.Builder
			for _, sp := range row {
				b.WriteString(sp.text)
			}
			got = append(got, b.String())
		}
		if strings.Join(got, "|") != tt.want {
			t.Errorf("flow(%+q, %d) = %+q, want %+q", tt.text, tt.width, strings.Join(got, "|"), tt.want)
		}
	}
}

func TestExpandAndWrappedLinks(t *testing.T) {
	target := &gedcom.Individual{ID: "X"}
	var ls []line
	for range 5 {
		ls = append(ls, textLine(span{text: "intro"}))
	}
	link := textLine(span{text: "Father", width: 8}, span{text: "a name that needs three rows"})
	link.target = target
	head := textLine(span{text: "Section heading"})
	head.heading = true
	ls = append(ls, link, head)

	rows := expand(ls, 20)
	if len(rows) != 9 {
		t.Fatalf("%d rows", len(rows))
	}
	for i, wantCont := range []bool{false, false, false, false, false, false, true, true, false} {
		if rows[i].cont != wantCont {
			t.Errorf("row %d: cont = %v", i, rows[i].cont)
		}
	}
	if rows[6].target != target || rows[7].target != target || !rows[8].heading || rows[6].heading {
		t.Error("rows keep the target, only the first is the heading")
	}

	// The rows of a link are one link: selected together, and shown
	// together when there is room.
	d := &docView{}
	d.set(rows)
	if len(d.links) != 1 || d.links[0] != 5 {
		t.Fatalf("links = %v", d.links)
	}
	d.ensureVisible(6)
	if d.offset != 2 {
		t.Errorf("offset = %d, want 2 to show all rows of the link", d.offset)
	}
	out := strings.Split(d.render(22, 6, defaultStyles()), "\n")
	if out[3] != "▸ Father  a name that" || out[4] != "          needs three" || out[5] != "          rows" {
		t.Errorf("render =\n%s", strings.Join(out, "\n"))
	}
}

func TestTextHelpers(t *testing.T) {
	if clamp(5, 0, 3) != 3 || clamp(-1, 0, 3) != 0 || clamp(2, 0, -1) != 0 {
		t.Error("clamp")
	}
	if fit("hello", 0) != "" || fit("hello", 10) != "hello" || lipgloss.Width(fit("hello world", 5)) != 5 {
		t.Error("fit")
	}
	if pad("ab", 4) != "ab  " || pad("abcdef", 4) != "abc…" || pad("x", 0) != "" {
		t.Errorf("pad: %q %q", pad("ab", 4), pad("abcdef", 4))
	}
	if got := pad("山田", 5); lipgloss.Width(got) != 5 {
		t.Errorf("pad wide = %q", got)
	}
	got := wrap("the quick brown fox jumps over the lazy dog", 15)
	want := []string{"the quick brown", "fox jumps over", "the lazy dog"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrap = %q", got)
	}
	got = wrap("first\n\nsecond", 20)
	if strings.Join(got, "|") != "first||second" {
		t.Errorf("wrap paragraphs = %q", got)
	}
	got = wrap(strings.Repeat("x", 25), 10)
	if len(got) != 3 || got[0] != strings.Repeat("x", 10) || got[2] != "xxxxx" {
		t.Errorf("wrap long word = %q", got)
	}
	if got := wrap("a b", 3); len(got) != 1 {
		t.Errorf("minimum width: %q", got)
	}
	if gendered(gedcom.SexMale, "m", "f", "n") != "m" || gendered(gedcom.SexFemale, "m", "f", "n") != "f" || gendered(gedcom.SexUnknown, "m", "f", "n") != "n" {
		t.Error("gendered")
	}
	if fixHeight("a\nb\nc", 2) != "a\nb" || fixHeight("a", 3) != "a\n\n" {
		t.Error("fixHeight")
	}
}
