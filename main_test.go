package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/tui"
	"github.com/mattn/go-runewidth"
)

var sample = filepath.Join("testdata", "family.ged")

type result struct {
	code   int
	stdout string
	stderr string
	tui    *tui.Options
	stdin  bool
}

func runCLI(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	var r result
	fake := func(doc *gedcom.Document, opts tui.Options, fromStdin bool) error {
		if doc == nil {
			t.Fatal("TUI started without a document")
		}
		r.tui = &opts
		r.stdin = fromStdin
		return nil
	}
	r.code = run(args, strings.NewReader(stdin), &out, &errOut, fake)
	r.stdout, r.stderr = out.String(), errOut.String()
	return r
}

func TestQueryFlag(t *testing.T) {
	r := runCLI(t, "", "-q", "surname:jones born:<1850", sample)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	want := "I12    Jones, Alice                     1842–\nI11    Jones, Henry                     1840–\nI10    Jones, Robert                    1815–\n"
	if r.stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, want)
	}
	if r.tui != nil {
		t.Error("batch mode must not start the TUI")
	}
}

func TestFlagsAfterFile(t *testing.T) {
	r := runCLI(t, "", sample, "-q", "john")
	if r.code != 0 || !strings.Contains(r.stdout, "Smith, John") || !strings.Contains(r.stdout, "Leeds, Yorkshire, England") {
		t.Errorf("exit %d stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestTreeFlags(t *testing.T) {
	r := runCLI(t, "", "-tree", "I7", "-gen", "2", sample)
	pad := strings.Repeat(" ", 26) // width of "Thomas Smith (1842–1910) ─"
	want := pad + "┌─ John Smith (1817–1880) ▸\nThomas Smith (1842–1910) ─┤\n" + pad + "└─ Ann Taylor (1820–1845)\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("exit %d stdout:\n%s\nwant:\n%s", r.code, r.stdout, want)
	}
	r = runCLI(t, "", "-desc", "I14", "-ascii", sample)
	if r.code != 0 || !strings.Contains(r.stdout, "`-- = Lucy King (1868–)  m. 1890") || !strings.Contains(r.stdout, "|-- Harold Smith (1891–)") {
		t.Errorf("descendants:\n%s", r.stdout)
	}
	r = runCLI(t, "", "-hourglass", "I3", "-gen", "2", sample)
	if r.code != 0 || !strings.Contains(r.stdout, "\n                                                          ├─ John Smith (1817–1880) ─┤\n") ||
		!strings.HasPrefix(r.stdout, "◂ Thomas Smith (1842–1910) ──── ⚭ Ann Taylor (1820–1845) ─┐") {
		t.Errorf("hourglass:\n%s", r.stdout)
	}
}

func TestLifespansFlag(t *testing.T) {
	r := runCLI(t, "", "-lifespans", "I3", sample)
	for _, want := range []string{"father     William Smith   ━━━━", "           John Smith ", "1817–1880  63 y", "lived  with John",
		"John's age", "⚭ marriage"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("no %q in:\n%s", want, r.stdout)
		}
	}
	if r.code != 0 || strings.Contains(r.stdout, "grandfather") {
		t.Errorf("exit %d, more than one generation:\n%s", r.code, r.stdout)
	}
	r = runCLI(t, "", "-lifespans", "I14", "-gen", "2", "-ascii", sample)
	if r.code != 0 || !strings.Contains(r.stdout, "half-aunt           Emma Smith") || !strings.Contains(r.stdout, ">= 19 y") ||
		strings.ContainsAny(r.stdout, "━┊⚭") {
		t.Errorf("two generations in ASCII:\n%s", r.stdout)
	}
	if r := runCLI(t, "", "-lifespans", "I99", sample); r.code != 1 || !strings.Contains(r.stderr, `no individual with ID "I99"`) {
		t.Errorf("unknown ID: exit %d, %s", r.code, r.stderr)
	}
}

func TestDiffFlag(t *testing.T) {
	// The newer version: a name changed, a person added to a family.
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	newer := strings.Replace(string(data), "1 NAME John /Smith/", "1 NAME Johann /Smith/", 1)
	newer = strings.Replace(newer, "0 TRLR", "0 @I99@ INDI\n1 NAME Jane /New/\n1 FAMC @F1@\n0 TRLR", 1)
	newer = strings.Replace(newer, "1 CHIL @I3@", "1 CHIL @I3@\n1 CHIL @I99@", 1)
	r := runCLI(t, newer, "-diff", sample, "-")
	want := `People
  + I99  Jane New
  ~ I3   Johann Smith (1817–1880)
         name: John Smith → Johann Smith
Families
  ~ F1  William Smith & Elizabeth Brown
        child added: Jane New
people: 1 changed, 1 added · families: 1 changed
`
	if r.code != 0 || r.stdout != want {
		t.Errorf("exit %d, stderr %q, stdout:\n%s\nwant:\n%s", r.code, r.stderr, r.stdout, want)
	}
	if r := runCLI(t, "", "-diff", sample, sample); r.code != 0 || r.stdout != "no differences\n" {
		t.Errorf("the same file: %q", r.stdout)
	}
}

func TestTimelineFlag(t *testing.T) {
	r := runCLI(t, "", "-timeline", "I14", sample)
	if r.code != 0 {
		t.Fatal(r.stderr)
	}
	for _, want := range []string{"Arthur Smith (I14)", "Birth  (Salford, Lancashire, England)", "Marriage with Lucy King  (age ~24)", "Birth of son Harold Smith", "from 1914 to 1918", "Military service  (France)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("timeline lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestRelateFlag(t *testing.T) {
	tests := []struct {
		ids  string
		want []string
	}{
		{"I20,I23", []string{"Dorothy Jones is Harold Smith's third cousin.", "Closest common ancestors: William Smith & Elizabeth Brown (4 and 4 generations up)."}},
		{"I22,I12", []string{"Alice Jones is Grace Hill's husband's aunt.", "Related through Walter Jones."}},
		{"I22,I15", []string{"No relationship found between Grace Hill and Friedrich Müller."}},
		{"I3, I3", []string{"John Smith and John Smith are the same person."}},
		{"I3,I1", []string{"William Smith is John Smith's father."}},
		{"I20,I21", []string{"Rose Smith is Harold Smith's adoptive sister.", "Closest common ancestors: Arthur Smith & Lucy King (1 and 1 generations up)."}},
	}
	for _, tt := range tests {
		r := runCLI(t, "", "-relate", tt.ids, sample)
		if r.code != 0 {
			t.Errorf("%s: exit %d %s", tt.ids, r.code, r.stderr)
		}
		for _, w := range tt.want {
			if !strings.Contains(r.stdout, w) {
				t.Errorf("%s: output lacks %q:\n%s", tt.ids, w, r.stdout)
			}
		}
	}
	if strings.Contains(runCLI(t, "", "-relate", "I3,I1", sample).stdout, "Closest common") {
		t.Error("direct ancestors need no common ancestor line")
	}
}

func TestEventsFlag(t *testing.T) {
	r := runCLI(t, "", "-events", "type:marr place:leeds", sample)
	want := "15 Jun 1815          Marriage           William Smith & Elizabeth Brown      Leeds, Yorkshire, England\n" +
		"1888                 Marriage           Walter Jones & Grace Hill            Leeds, Yorkshire, England\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("exit %d stdout:\n%s\nwant:\n%s", r.code, r.stdout, want)
	}
	all := runCLI(t, "", "-events", "", "-all", sample)
	if n := strings.Count(all.stdout, "\n"); n != 54 {
		t.Errorf("-events '' -all printed %d events", n)
	}
	if !strings.Contains(all.stdout, "Occupation: Weaver") || !strings.Contains(all.stdout, "—  ") {
		t.Errorf("attributes and undated events:\n%s", all.stdout)
	}
	vital := runCLI(t, "", "-events", "", sample)
	if n := strings.Count(vital.stdout, "\n"); n != 46 {
		t.Errorf("-events '' printed %d events", n)
	}
}

func TestStatsFlag(t *testing.T) {
	r := runCLI(t, "", "-stats", sample)
	for _, want := range []string{"File:          family.ged (UTF-8, GEDCOM 5.5.1)", "Individuals:   24 (10 male, 13 female, 1 other/unknown)", "Years:         1790–1940", "Generations:   5", "Avg lifespan:  50.0 years (10 people)", "Top surnames:  Smith (10), Jones (5)",
		"Collapse:      0.0% of ancestor positions repeat an ancestor (no one has an ancestor more than once)\nWarnings:      0"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stats lack %q:\n%s", want, r.stdout)
		}
	}
	r = runCLI(t, "0 HEAD\n0 @I1@ INDI\n1 FAMS @F1@\n", "-stats", "-")
	if !strings.Contains(r.stdout, "Warnings:      2") || !strings.Contains(r.stdout, "refers to missing family @F1@") {
		t.Errorf("stats with warnings:\n%s", r.stdout)
	}
}

func TestPlacesFlag(t *testing.T) {
	r := runCLI(t, "", "-places", sample)
	if r.code != 0 {
		t.Fatal(r.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	if len(lines) != 17 {
		t.Fatalf("%d lines:\n%s", len(lines), r.stdout)
	}
	row := func(name string, events int, ev string, people int, pe string) string {
		return fmt.Sprintf("%-40s %5d %-6s %5d %s", name, events, ev, people, pe)
	}
	for i, want := range map[int]string{
		0:  row("Canada", 1, "event", 1, "person"),
		3:  row("England", 19, "events", 11, "people"),
		10: row("    Leeds", 9, "events", 6, "people"),
		11: row("      St Peter's", 1, "event", 1, "person"),
		16: row("  Köln", 1, "event", 1, "person"),
	} {
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q", i, lines[i], want)
		}
	}
}

func TestColumnsWithWideAndDecomposedText(t *testing.T) {
	// Hanna's name and places are decomposed (NFD): their combining marks
	// take no column. Taro's are Japanese: every character takes two.
	ged := strings.Join([]string{
		"0 HEAD",
		"0 @I1@ INDI", "1 NAME Hans /M\u00FCller/", "1 BIRT", "2 DATE 1900", "2 PLAC Berlin",
		"0 @I2@ INDI", "1 NAME Hanna /Mu\u0308ller/", "1 BIRT", "2 DATE 1901", "2 PLAC Mu\u0308nchen",
		"1 RESI", "2 DATE Fru\u0308hjahr 1920", "2 PLAC Mu\u0308nchen", // a date shown as written
		"0 @I3@ INDI", "1 NAME \u592A\u90CE /\u7530\u4E2D/", "1 BIRT", "2 DATE 1902", "2 PLAC \u6771\u4EAC",
		"0 TRLR", "",
	}, "\n")
	// columns returns, for every line containing one of subs, the display
	// column at which it starts.
	columns := func(out string, subs ...string) []int {
		var cols []int
		for _, line := range strings.Split(out, "\n") {
			for _, sub := range subs {
				if i := strings.Index(line, sub); i >= 0 {
					cols = append(cols, runewidth.StringWidth(line[:i]))
					break
				}
			}
		}
		return cols
	}
	places := []string{"Berlin", "Mu\u0308nchen", "\u6771\u4EAC"}
	for _, tt := range []struct {
		args []string
		subs []string
		col  int // where every line has the column
		n    int // number of lines with it
	}{
		{[]string{"-q", ""}, places, 53, 3},
		{[]string{"-events", "", "-all"}, places, 77, 4},
		{[]string{"-places"}, []string{" event"}, 46, 3},
		{[]string{"-timeline", "I2"}, []string{"Birth", "Residence"}, 25, 2},
	} {
		r := runCLI(t, ged, append(tt.args, "-")...)
		if r.code != 0 {
			t.Fatalf("%v: exit %d: %s", tt.args, r.code, r.stderr)
		}
		cols := columns(r.stdout, tt.subs...)
		ok := len(cols) == tt.n
		for _, c := range cols {
			ok = ok && c == tt.col
		}
		if !ok {
			t.Errorf("%v: columns %v, want %d lines at %d:\n%s", tt.args, cols, tt.n, tt.col, r.stdout)
		}
	}
}

func TestExportFlag(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "family.ged")
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	// By default the export goes next to the input, with the format's
	// extension; the format's name is not case-sensitive.
	r := runCLI(t, "", "-export", "GEXF", input)
	if r.code != 0 || r.stderr != "goged: exported 24 people as GEXF to "+filepath.Join(dir, "family.gexf")+"\n" || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if g := read("family.gexf"); strings.Count(g, "<node ") != 24 || !strings.Contains(g, "All 24 people of family.ged") {
		t.Errorf("family.gexf:\n%.400s", g)
	}

	r = runCLI(t, "", "-export", "html", input)
	if r.code != 0 || !strings.HasPrefix(read("family.html"), "<!DOCTYPE html>") {
		t.Fatalf("html: exit %d, %q", r.code, r.stderr)
	}

	// Only the people connected to a pivot, to a file of one's choice;
	// flags may follow the file name.
	out := filepath.Join(dir, "smith.pdf")
	r = runCLI(t, "", input, "-export", "pdf", "-pivot", "I20", "-o", out)
	if r.code != 0 || !strings.HasPrefix(read("smith.pdf"), "%PDF-") {
		t.Fatalf("pdf: exit %d, %q", r.code, r.stderr)
	}

	// To standard output, also when reading from standard input.
	r = runCLI(t, string(data), "-export", "gexf", "-pivot", "I20", "-o", "-", "-")
	if r.code != 0 || strings.Count(r.stdout, "<node ") != 24 || !strings.Contains(r.stdout, "connected to Harold Smith (I20)") || r.stderr != "" {
		t.Errorf("stdout: exit %d, stderr %q", r.code, r.stderr)
	}

	for _, tt := range []struct {
		stdin string
		args  []string
		code  int
		err   string
	}{
		{"", []string{"-export", "svg", input}, 2, `unknown export format "svg" (formats: gexf, pdf, html)`},
		{"", []string{"-export", "gexf", "-pivot", "I999", input}, 1, `no individual with ID "I999"`},
		{"", []string{"-pivot", "I20", input}, 2, "-pivot and -o go with -export"},
		{"", []string{"-o", "x.gexf", input}, 2, "-pivot and -o go with -export"},
		{string(data), []string{"-export", "gexf", "-"}, 2, "name the export with -o when reading from standard input"},
		{"", []string{"-export", "gexf", "-o", input, input}, 2, "would replace the input file"},
		{"", []string{"-export", "gexf", "-o", filepath.Join(dir, "missing", "x.gexf"), input}, 1, "export failed:"},
	} {
		r := runCLI(t, tt.stdin, tt.args...)
		if r.code != tt.code || !strings.Contains(r.stderr, tt.err) {
			t.Errorf("%v: exit %d, stderr %q; want %d, %q", tt.args, r.code, r.stderr, tt.code, tt.err)
		}
	}
	if read("family.ged") != string(data) {
		t.Error("the input file changed")
	}
}

func TestStdin(t *testing.T) {
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	r := runCLI(t, string(data), "-q", "rose", "-")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "I21    Smith, Rose") {
		t.Errorf("stdin query: %d %q %q", r.code, r.stdout, r.stderr)
	}
	r = runCLI(t, string(data), "-")
	if r.tui == nil || !r.stdin || r.tui.Title != "stdin" || r.tui.Path != "" || r.tui.Load != nil {
		t.Errorf("TUI from stdin: %+v %v", r.tui, r.stdin)
	}
}

func TestStartsTUI(t *testing.T) {
	r := runCLI(t, "", "-person", "I3", "-gen", "6", "-ascii", "-no-mouse", sample)
	if r.code != 0 || r.tui == nil {
		t.Fatalf("exit %d, tui %v, stderr %s", r.code, r.tui, r.stderr)
	}
	// The browser can load the file again.
	got := *r.tui
	if got.Load == nil {
		t.Fatal("no Load")
	}
	if doc, err := got.Load(); err != nil || len(doc.Individuals) != 24 {
		t.Errorf("Load: %v", err)
	}
	got.Load = nil
	want := tui.Options{Title: "family.ged", StartPerson: "I3", Generations: 6, ASCII: true, NoMouse: true, Path: sample}
	if !reflect.DeepEqual(got, want) || r.stdin {
		t.Errorf("options = %+v, want %+v", got, want)
	}
	// What is restricted stays hidden when the file is loaded again,
	// unless it was shown.
	for _, show := range []bool{false, true} {
		args := []string{filepath.Join("testdata", "Muster_GEDCOM_UTF-8.ged")}
		if show {
			args = append(args, "-show-restricted")
		}
		doc, err := runCLI(t, "", args...).tui.Load()
		if err != nil || (doc.Redactions > 0) == show {
			t.Errorf("show %v: %v, %d redactions", show, err, doc.Redactions)
		}
	}

	var errOut bytes.Buffer
	failing := func(*gedcom.Document, tui.Options, bool) error { return errors.New("no terminal") }
	if code := run([]string{sample}, strings.NewReader(""), &bytes.Buffer{}, &errOut, failing); code != 1 || !strings.Contains(errOut.String(), "no terminal") {
		t.Errorf("TUI failure: exit %d %q", code, errOut.String())
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		args   []string
		code   int
		stderr string
	}{
		{nil, 2, "Usage:"},
		{[]string{"a.ged", "b.ged"}, 2, "Usage:"},
		{[]string{"-bogus", sample}, 2, "flag provided but not defined"},
		{[]string{filepath.Join("testdata", "missing.ged")}, 1, "missing.ged"},
		{[]string{"-tree", "I99", sample}, 1, `no individual with ID "I99"`},
		{[]string{"-hourglass", "I99", sample}, 1, `no individual with ID "I99"`},
		{[]string{"-diff", filepath.Join("testdata", "missing.ged"), sample}, 1, "missing.ged"},
		{[]string{"-desc", "I99", sample}, 1, `no individual with ID "I99"`},
		{[]string{"-timeline", "I99", sample}, 1, `no individual with ID "I99"`},
		{[]string{"-relate", "I1", sample}, 2, "two IDs separated by a comma"},
		{[]string{"-relate", "I1,I99", sample}, 1, `no individual with ID "I99"`},
		{[]string{"-q", "foo:bar", sample}, 2, `unknown field "foo"`},
		{[]string{"-events", "sex:m", sample}, 2, "not available for events"},
	}
	for _, tt := range tests {
		r := runCLI(t, "", tt.args...)
		if r.code != tt.code || !strings.Contains(r.stderr, tt.stderr) {
			t.Errorf("%v: exit %d stderr %q; want %d %q", tt.args, r.code, r.stderr, tt.code, tt.stderr)
		}
		if r.tui != nil {
			t.Errorf("%v: TUI must not start on errors", tt.args)
		}
	}
	r := runCLI(t, "not a gedcom file", "-")
	if r.code != 1 || !strings.Contains(r.stderr, "no records found") {
		t.Errorf("garbage input: %d %q", r.code, r.stderr)
	}
}

func TestHelpAndVersion(t *testing.T) {
	r := runCLI(t, "", "-h")
	if r.code != 0 || !strings.Contains(r.stderr, "goged — browse GEDCOM") || !strings.Contains(r.stderr, "-relate") {
		t.Errorf("-h: %d %q", r.code, r.stderr)
	}
	r = runCLI(t, "", "-licenses")
	if r.code != 0 {
		t.Errorf("-licenses: exit %d", r.code)
	}
	for _, want := range []string{"Natural Earth", "public domain", "Martian Mono", "SIL OPEN FONT LICENSE Version 1.1",
		"github.com/charmbracelet/bubbletea", "codeberg.org/go-pdf/fpdf"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("-licenses lacks %q", want)
		}
	}
	r = runCLI(t, "", "-version")
	if r.code != 0 || r.stdout != "goged dev\n" {
		t.Errorf("-version: %d %q", r.code, r.stdout)
	}
}

// TestMusterFile runs the batch flags against the GEDCOM-L sample file,
// which uses many GEDCOM 5.5.1 features and German-language conventions.
func TestMusterFile(t *testing.T) {
	muster := filepath.Join("testdata", "Muster_GEDCOM_UTF-8.ged")
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"-stats"}, []string{"(UTF-8, GEDCOM 5.5.1)", "Individuals:   37 (21 male, 16 female", "Families:      18", "Places:        23", "Sources:       11", "Years:         1794–2005",
			"Restricted:    2 hidden (confidential or private; see -show-restricted)", "Warnings:      0",
			"Collapse:      0.0% of ancestor positions repeat an ancestor (no one has an ancestor more than once)"}},
		// Karl Junior's birth and christening are confidential and private.
		{[]string{"-q", "surname:muller"}, []string{"I22    Müller, Karl Junior              –1980", "I37    Müller, Roswitha"}},
		{[]string{"-show-restricted", "-q", "surname:muller"}, []string{"I22    Müller, Karl Junior              1933–1980"}},
		{[]string{"-timeline", "I22"}, []string{"Karl Müller Junior (I22)\n  7 Jun 1935             Birth of sister"}},
		{[]string{"-timeline", "I22", "-show-restricted"}, []string{"1 May 1933             Birth  (Wenningen, Hochsauerlandkreis, Nordrhein-Westfalen, Deutschland; confidential)"}},
		{[]string{"-q", "given:desire"}, []string{"I7     Mustermann, Desiré Jeanette"}},
		// Maria's "von" is recorded as SPFX and is not sorted; Erich Karl's is
		// part of the surname. Titles (NPFX) are not given names.
		{[]string{"-q", "stradonitz"}, []string{"I13    Stradonitz, Maria von            1910–\nI30    von Stradonitz, Erich Karl       1855–\n"}},
		{[]string{"-q", "freiherr"}, []string{"I30    von Stradonitz, Erich Karl"}},
		{[]string{"-q", "elton"}, []string{"I32    Dwight, Reginald Kenneth  (as Elton John)\n"}},
		{[]string{"-q", "born:1794"}, []string{"I29    Frantz, Eva"}},
		// Brzozowo is the Polish name in the place record of Brosowo.
		{[]string{"-q", "place:brzozowo"}, []string{"I1     Mustermann, Max Manfred"}},
		{[]string{"-places"}, []string{"  Berlin                                     6 events     6 people\n    Tempelhof                                5 events     5 people\n"}},
		{[]string{"-relate", "I3,I7"}, []string{"Desiré Jeanette Mustermann is Leon Mustermann's half-sister."}},
		{[]string{"-relate", "I6,I19"}, []string{"Gerold Freiwein is Markus Schüchter's adoptive father."}},
		{[]string{"-timeline", "I37"}, []string{"Christening  (Wenningen, Hochsauerlandkreis, Nordrhein-Westfalen, Deutschland; religion: evangelisch; godparents: Max Herbert Mustermann, Musiker)"}},
		{[]string{"-timeline", "I11"}, []string{"Divorce from Brigitte Gutedel filed  (Tempelhof", "Divorce from Brigitte Gutedel  (Tempelhof", "Jul 1997               Pilgerreise nach Rom with Brigitte Gutedel  (age ~24)"}},
		{[]string{"-desc", "I30"}, []string{"Freiherr Erich Karl von Stradonitz (1855–)\n└── ⚭ Freifrau Charlotte v. Musterow (1877–)  m. 1892\n"}},
		{[]string{"-tree", "I13"}, []string{"Freifrau Charlotte v. Musterow (1877–)"}},
		{[]string{"-timeline", "I6"}, []string{"Adoption  (by Gerold Freiwein; Köln"}},
		{[]string{"-timeline", "I29"}, []string{"24 Oct 1794            Birth  (3. Brumaire III; Nohfelden"}},
		{[]string{"-timeline", "I1"}, []string{"  Apr 1958–Mar 1961      Education  (Werkzeugschlosser Lehre;"}},
		{[]string{"-events", "type:marr", "-all"}, []string{"4 Jul 1958           Civil marriage", "5 Jul 1958           Religious marriage"}},
		{[]string{"-relate", "I22,I1"}, []string{"Max Manfred Mustermann is Karl Müller Junior's co-father-in-law.\nRelated through Friedhelm Müller.\n"}},
	}
	if r := runCLI(t, "", "-q", "given:freiherr", muster); r.stdout != "" {
		t.Errorf("a title matched as given name: %q", r.stdout)
	}
	// A residence "after May 1996" is no evidence for 2005.
	if r := runCLI(t, "", "-q", "year:2005", muster); r.stdout != "I32    Dwight, Reginald Kenneth\nI33    Furnish, David\n" {
		t.Errorf("year:2005:\n%s", r.stdout)
	}
	// Mustermann and Musterow share the Soundex code M236.
	if r := runCLI(t, "", "-q", "~musterman", muster); strings.Contains(r.stdout, "Musterow") || !strings.Contains(r.stdout, "Mustermann, Max Manfred") {
		t.Errorf("~musterman:\n%s", r.stdout)
	}
	for _, c := range cases {
		r := runCLI(t, "", append(c.args, muster)...)
		if r.code != 0 || r.stderr != "" {
			t.Errorf("%v: exit %d, stderr %q", c.args, r.code, r.stderr)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(r.stdout, w) {
				t.Errorf("%v: missing %q in\n%s", c.args, w, r.stdout)
			}
		}
	}
}
