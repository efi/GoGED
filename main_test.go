package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/tui"
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
	for _, want := range []string{"File:          family.ged (UTF-8, GEDCOM 5.5.1)", "Individuals:   24 (10 male, 13 female, 1 other/unknown)", "Years:         1790–1940", "Generations:   5", "Avg lifespan:  50.0 years (10 people)", "Top surnames:  Smith (10), Jones (5)", "Warnings:      0"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stats lack %q:\n%s", want, r.stdout)
		}
	}
	r = runCLI(t, "0 HEAD\n0 @I1@ INDI\n1 FAMS @F1@\n", "-stats", "-")
	if !strings.Contains(r.stdout, "Warnings:      2") || !strings.Contains(r.stdout, "refers to missing family @F1@") {
		t.Errorf("stats with warnings:\n%s", r.stdout)
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
	if r.tui == nil || !r.stdin || r.tui.Title != "stdin" {
		t.Errorf("TUI from stdin: %+v %v", r.tui, r.stdin)
	}
}

func TestStartsTUI(t *testing.T) {
	r := runCLI(t, "", "-person", "I3", "-gen", "6", "-ascii", sample)
	if r.code != 0 || r.tui == nil {
		t.Fatalf("exit %d, tui %v, stderr %s", r.code, r.tui, r.stderr)
	}
	want := tui.Options{Title: "family.ged", StartPerson: "I3", Generations: 6, ASCII: true}
	if *r.tui != want || r.stdin {
		t.Errorf("options = %+v, want %+v", *r.tui, want)
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
	r = runCLI(t, "", "-version")
	if r.code != 0 || r.stdout != "goged dev\n" {
		t.Errorf("-version: %d %q", r.code, r.stdout)
	}
}
