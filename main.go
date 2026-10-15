// Command goged is a terminal browser for GEDCOM genealogy files.
//
// Run it with a GEDCOM file to open the interactive browser, or use one of
// the batch flags to print search results, trees, timelines, relationships,
// events or statistics.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/diff"
	"github.com/efi/goged/export"
	"github.com/efi/goged/fonts"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/licenses"
	"github.com/efi/goged/search"
	"github.com/efi/goged/tui"
	"github.com/efi/goged/worldmap"
	"github.com/mattn/go-runewidth"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, runTUI))
}

// tuiRunner starts the interactive interface; tests replace it.
type tuiRunner func(doc *gedcom.Document, opts tui.Options, fromStdin bool) error

func runTUI(doc *gedcom.Document, opts tui.Options, fromStdin bool) error {
	progOpts := []tea.ProgramOption{tea.WithAltScreen()}
	if !opts.NoMouse {
		progOpts = append(progOpts, tea.WithMouseCellMotion())
	}
	if fromStdin {
		// The file came through stdin, so read keys from the terminal.
		progOpts = append(progOpts, tea.WithInputTTY())
	}
	_, err := tea.NewProgram(tui.New(doc, opts), progOpts...).Run()
	return err
}

const usageText = `goged — browse GEDCOM genealogy files in the terminal

Usage:
  goged [flags] FILE.ged                  open the interactive browser
  goged -q QUERY FILE.ged                 print matching people and exit
  goged -export FORMAT [-pivot ID] [-o OUT] FILE.ged
                                          export as gexf, pdf or html and exit
  goged -diff OLD.ged FILE.ged            print what changed since OLD.ged
  goged -checks all [-person ID] FILE.ged print the findings of the checks and hints

Add -json to a flag that prints to get the output as JSON.

Use - as FILE to read from standard input.

Flags:
`

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, startTUI tuiRunner) int {
	fs := flag.NewFlagSet("goged", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		query    = fs.String("q", "", "print the people matching `QUERY` (see the help screen for the syntax)")
		pedigree = fs.String("tree", "", "print the pedigree chart of the person with `ID`")
		desc     = fs.String("desc", "", "print the descendant chart of the person with `ID`")
		hourglas = fs.String("hourglass", "", "print the hourglass chart of the person with `ID`: descendants left, ancestors right")
		lifespan = fs.String("lifespans", "", "print the lifespans of the family of the person with `ID` on a time axis: parents, siblings, partners and children (more with -gen)")
		timeline = fs.String("timeline", "", "print the timeline of the person with `ID`")
		relate   = fs.String("relate", "", "print how two people are related, e.g. -relate I1,I20 (`ID1,ID2`)")
		events   = fs.String("events", "", "print events matching `FILTER`, e.g. \"type:marr 1850..1870\"")
		allEv    = fs.Bool("all", false, "with -events: include all events, not only births, marriages and deaths")
		places   = fs.Bool("places", false, "print all places as a hierarchy with the number of events and people")
		stats    = fs.Bool("stats", false, "print statistics and warnings about the file")
		gens     = fs.Int("gen", 4, "number of `generations` shown in trees; with -lifespans, how many each way (default 1)")
		ascii    = fs.Bool("ascii", false, "draw trees with ASCII characters only")
		noMouse  = fs.Bool("no-mouse", false, "leave the mouse to the terminal, to select text as usual")
		person   = fs.String("person", "", "open the browser at the person with `ID`; with -checks, mark them for the hint on unknown parents")
		showRes  = fs.Bool("show-restricted", false, "show data marked confidential or private (RESN), which is hidden by default")
		showVer  = fs.Bool("version", false, "print the version and exit")
		showLic  = fs.Bool("licenses", false, "print the licenses of the map data and the third-party software")
		exportAs = fs.String("export", "", "export as `FORMAT` (gexf, pdf or html) and exit")
		pivot    = fs.String("pivot", "", "with -export: only the person with `ID` and everybody connected to them")
		output   = fs.String("o", "", "with -export: write to `FILE` (- for standard output); default: the input file with the format's extension")
		diffWith = fs.String("diff", "", "print what changed from the older version in `OLD.ged` to the file given, records paired by their IDs")
		checks   = fs.String("checks", "", "print the findings of the plausibility checks and hints: all, checks, hints, or `IDs` with thresholds, e.g. old-age=100,similar-places=somewhat-resembling")
		asJSON   = fs.Bool("json", false, "with -q, -tree, -desc, -hourglass, -lifespans, -timeline, -relate, -events, -places, -stats, -checks or -diff: print JSON")
	)
	fs.Usage = func() {
		fmt.Fprint(stderr, usageText)
		fs.PrintDefaults()
	}

	// Allow flags after the file name: parse, take one positional
	// argument, and continue with the rest.
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	if *showVer {
		fmt.Fprintln(stdout, "goged", version)
		return 0
	}
	if *showLic {
		fmt.Fprintf(stdout, "%s\n\n%s\n%s", worldmap.License, fonts.License, licenses.ThirdParty)
		return 0
	}
	if (set["pivot"] || set["o"]) && !set["export"] {
		fmt.Fprintln(stderr, "goged: -pivot and -o go with -export")
		return 2
	}
	if *asJSON && !slices.ContainsFunc(jsonFlags, func(f string) bool { return set[f] }) {
		fmt.Fprintln(stderr, "goged: -json goes with -"+strings.Join(jsonFlags, ", -"))
		return 2
	}
	if len(positional) != 1 {
		fs.Usage()
		return 2
	}

	path := positional[0]
	loadFile := func(p string) (*gedcom.Document, error) {
		doc, err := gedcom.ParseFile(p)
		if err != nil {
			return nil, err
		}
		if !*showRes {
			doc = doc.Redacted()
		}
		return doc, nil
	}
	// load reads the file; the browser uses it again to reload the file.
	load := func() (*gedcom.Document, error) { return loadFile(path) }
	var (
		doc *gedcom.Document
		err error
	)
	if path == "-" {
		doc, err = gedcom.Parse(stdin)
		if err == nil && !*showRes {
			doc = doc.Redacted()
		}
	} else {
		doc, err = load()
	}
	if err != nil {
		fmt.Fprintf(stderr, "goged: %s: %v\n", path, err)
		return 1
	}

	chartOpts := chart.Options{Generations: *gens, ASCII: *ascii}
	lookup := func(id string) (*gedcom.Individual, bool) {
		ind := doc.Individual(id)
		if ind == nil {
			fmt.Fprintf(stderr, "goged: no individual with ID %q\n", id)
			return nil, false
		}
		return ind, true
	}

	// show prints the output as text, or with -json its data.
	show := func(text func(), data func() any) int {
		if !*asJSON {
			text()
			return 0
		}
		if err := writeJSON(stdout, data()); err != nil {
			fmt.Fprintf(stderr, "goged: %v\n", err)
			return 1
		}
		return 0
	}
	// chartOf prints the chart of the person with id.
	chartOf := func(id string, draw func(*gedcom.Individual) *chart.Chart, ancestors, descendants bool) int {
		ind, ok := lookup(id)
		if !ok {
			return 1
		}
		return show(func() { fmt.Fprint(stdout, draw(ind).String()) },
			func() any { return treeJSON(ind, chartOpts, ancestors, descendants) })
	}

	switch {
	case set["q"]:
		results, err := search.NewIndex(doc).SearchString(*query)
		if err != nil {
			fmt.Fprintf(stderr, "goged: %v\n", err)
			return 2
		}
		return show(func() { printResults(results, stdout) }, func() any { return queryJSON(results) })
	case set["tree"]:
		return chartOf(*pedigree, func(ind *gedcom.Individual) *chart.Chart { return chart.Pedigree(ind, chartOpts) }, true, false)
	case set["desc"]:
		descOpts := chartOpts
		descOpts.MaxLabel = 1 << 16 // one person per line: no need to shorten labels
		return chartOf(*desc, func(ind *gedcom.Individual) *chart.Chart { return chart.Descendants(ind, descOpts) }, false, true)
	case set["hourglass"]:
		return chartOf(*hourglas, func(ind *gedcom.Individual) *chart.Chart { return chart.Hourglass(ind, chartOpts) }, true, true)
	case set["lifespans"]:
		ind, ok := lookup(*lifespan)
		if !ok {
			return 1
		}
		opts := chart.LifespanOptions{Generations: 1, ASCII: *ascii}
		if set["gen"] {
			opts.Generations = *gens
		}
		c := chart.Lifespans(ind, opts)
		return show(func() { fmt.Fprint(stdout, c.String()) }, func() any { return lifespansJSON(c, opts.Generations) })
	case set["timeline"]:
		ind, ok := lookup(*timeline)
		if !ok {
			return 1
		}
		return show(func() { printTimeline(ind, stdout) }, func() any { return timelineJSON(ind) })
	case set["relate"]:
		a, b, found := strings.Cut(*relate, ",")
		if !found {
			fmt.Fprintln(stderr, "goged: -relate expects two IDs separated by a comma")
			return 2
		}
		indA, ok1 := lookup(strings.TrimSpace(a))
		indB, ok2 := lookup(strings.TrimSpace(b))
		if !ok1 || !ok2 {
			return 1
		}
		return show(func() { printRelationship(indA, indB, stdout) }, func() any { return relationshipJSON(indA, indB) })
	case set["events"]:
		q, err := search.ParseEventQuery(*events)
		if err != nil {
			fmt.Fprintf(stderr, "goged: %v\n", err)
			return 2
		}
		list := search.NewEventIndex(doc.Events()).Filter(q, !*allEv)
		return show(func() { printEvents(list, stdout) }, func() any { return eventsJSON(list) })
	case set["checks"]:
		var marked *gedcom.Individual
		if set["person"] {
			var ok bool
			if marked, ok = lookup(*person); !ok {
				return 1
			}
		}
		return runChecks(doc, *checks, marked, *asJSON, stdout, stderr)
	case set["export"]:
		return runExport(doc, path, *exportAs, *pivot, *output, stdout, stderr)
	case set["diff"]:
		old, err := loadFile(*diffWith)
		if err != nil {
			fmt.Fprintf(stderr, "goged: %s: %v\n", *diffWith, err)
			return 1
		}
		r := diff.Compare(old, doc)
		return show(func() { printDiff(r, stdout) }, func() any { return diffJSON(r) })
	case *places:
		return show(func() { printPlaces(doc, stdout) }, func() any { return placesJSON(genealogy.Places(doc)) })
	case *stats:
		return show(func() { printStats(doc, filepath.Base(path), stdout) }, func() any { return statsJSON(doc, filepath.Base(path)) })
	}

	opts := tui.Options{Title: filepath.Base(path), StartPerson: *person, Generations: *gens, ASCII: *ascii, NoMouse: *noMouse}
	if path == "-" {
		opts.Title = "stdin"
	} else {
		opts.Path, opts.Load = path, load
	}
	if err := startTUI(doc, opts, path == "-"); err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 1
	}
	return 0
}

// runExport exports doc, read from input, in the format named, to output
// or by default to the input's path with the format's extension.
func runExport(doc *gedcom.Document, input, format, pivot, output string, stdout, stderr io.Writer) int {
	title := filepath.Base(input)
	if input == "-" {
		title = "stdin"
	}
	formats := tui.ExportFormats(doc, tui.Options{Title: title})
	var names []string
	f := -1
	for i, ef := range formats {
		names = append(names, strings.ToLower(ef.Name))
		if strings.EqualFold(ef.Name, format) {
			f = i
		}
	}
	if f < 0 {
		fmt.Fprintf(stderr, "goged: unknown export format %q (formats: %s)\n", format, strings.Join(names, ", "))
		return 2
	}
	opts := export.Options{Title: title, Generated: time.Now()}
	if pivot != "" {
		root := doc.Individual(pivot)
		if root == nil {
			fmt.Fprintf(stderr, "goged: no individual with ID %q\n", pivot)
			return 1
		}
		opts.Scope, opts.Root = export.Component, root
	}
	sel, err := export.Select(doc, opts)
	if err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 1
	}

	path := output
	if path == "" {
		if input == "-" {
			fmt.Fprintln(stderr, "goged: name the export with -o when reading from standard input (- writes to standard output)")
			return 2
		}
		path = strings.TrimSuffix(input, filepath.Ext(input)) + formats[f].Extension
	}
	if path == "-" {
		err = formats[f].Write(stdout, sel, opts)
	} else {
		if a, b := absPath(path), absPath(input); a == b {
			fmt.Fprintf(stderr, "goged: the export would replace the input file %s\n", input)
			return 2
		}
		err = export.WriteFile(path, formats[f], sel, opts)
	}
	if err != nil {
		fmt.Fprintf(stderr, "goged: export failed: %v\n", err)
		return 1
	}
	if path != "-" {
		fmt.Fprintf(stderr, "goged: exported %d people as %s to %s\n", len(sel.People), formats[f].Name, path)
	}
	return 0
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// jsonFlags are the flags whose output -json gives as data.
var jsonFlags = []string{"q", "tree", "desc", "hourglass", "lifespans", "timeline", "relate", "events", "places", "stats", "checks", "diff"}

func printResults(results []search.Result, stdout io.Writer) {
	for _, r := range results {
		ind := r.Individual
		place := ""
		if b := ind.FirstDatedEvent(gedcom.BirthTags...); b != nil {
			place = b.Place.String()
		}
		line := strings.TrimRight(pad(ind.ID, 6)+" "+pad(ind.SortName(), 32)+" "+pad(ind.Lifespan(), 12)+" "+place, " ")
		if r.NameIndex > 0 && r.NameIndex < len(ind.Names) {
			// The query matched an alternate name; show it.
			line += "  (as " + ind.Names[r.NameIndex].String() + ")"
		}
		fmt.Fprintln(stdout, line)
	}
}

// pad fills s with spaces to width terminal columns. Unlike %-*s, which
// counts characters, it measures what a terminal shows: combining marks,
// as in decomposed (NFD) text, take no column and East Asian wide
// characters take two.
func pad(s string, width int) string {
	if w := runewidth.StringWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

func printTimeline(ind *gedcom.Individual, w io.Writer) {
	fmt.Fprintf(w, "%s (%s)\n", ind.DisplayName(), ind.ID)
	for _, e := range genealogy.Timeline(ind, genealogy.TimelineOptions{Relatives: true}) {
		date, dateRest := "—", ""
		if !e.Event.Date.IsZero() {
			date, dateRest = e.Event.Date.Fit(22)
		}
		title := e.Title()
		var rest []string
		if d := e.Event.Detail(); d != "" {
			rest = append(rest, d)
		}
		if dateRest != "" {
			rest = append(rest, dateRest)
		}
		if p := e.Event.Place.String(); p != "" {
			rest = append(rest, p)
		}
		if e.HasAge {
			rest = append(rest, "age "+e.Age.String())
		}
		if r := e.Event.Restriction; r != "" {
			rest = append(rest, r)
		}
		if e.Own() {
			for _, f := range e.Event.Facts() {
				if f.Label != "address" {
					rest = append(rest, f.Label+": "+f.Value)
				}
			}
		}
		lineText := "  " + pad(date, 22) + " " + title
		if len(rest) > 0 {
			lineText += "  (" + strings.Join(rest, "; ") + ")"
		}
		fmt.Fprintln(w, lineText)
	}
}

func printRelationship(a, b *gedcom.Individual, w io.Writer) {
	r := genealogy.Relate(a, b)
	switch r.Kind {
	case genealogy.KindNone:
		fmt.Fprintf(w, "No relationship found between %s and %s.\n", a.DisplayName(), b.DisplayName())
		return
	case genealogy.KindSelf:
		fmt.Fprintf(w, "%s and %s are the same person.\n", a.DisplayName(), b.DisplayName())
		return
	}
	fmt.Fprintf(w, "%s is %s's %s.\n", b.DisplayName(), a.DisplayName(), r.Description)
	if (r.Kind == genealogy.KindBlood || r.Kind == genealogy.KindAdoptive) && len(r.CommonAncestors) > 0 && r.UpA > 0 && r.UpB > 0 {
		var names []string
		for _, c := range r.CommonAncestors {
			names = append(names, c.DisplayName())
		}
		fmt.Fprintf(w, "Closest common ancestors: %s (%d and %d generations up).\n", strings.Join(names, " & "), r.UpA, r.UpB)
	}
	if r.Via != nil && r.Kind == genealogy.KindMarriage {
		fmt.Fprintf(w, "Related through %s.\n", r.Via.DisplayName())
	}
}

func printEvents(list []*gedcom.Event, stdout io.Writer) {
	for _, ev := range list {
		date := "—"
		if ev.Date.IsValid() {
			date, _ = ev.Date.Fit(20)
		}
		var names []string
		for _, p := range ev.Principals() {
			names = append(names, p.DisplayName())
		}
		label := ev.Label()
		if d := ev.Detail(); d != "" && ev.Class() == gedcom.ClassAttribute {
			label += ": " + d
		}
		fmt.Fprintln(stdout, strings.TrimRight(pad(date, 20)+" "+pad(label, 18)+" "+pad(strings.Join(names, " & "), 36)+" "+ev.Place.String(), " "))
	}
}

func printPlaces(doc *gedcom.Document, w io.Writer) {
	word := func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	}
	genealogy.Places(doc).Walk(func(n *genealogy.PlaceNode) bool {
		if n.Depth > 0 {
			name := strings.Repeat("  ", n.Depth-1) + n.Name
			line := pad(name, 40) + fmt.Sprintf(" %5d %-6s %5d %s", n.Count(), word(n.Count(), "event", "events"), n.People(), word(n.People(), "person", "people"))
			fmt.Fprintln(w, line)
		}
		return true
	})
}

// printDiff prints the differences: the header's, then the people, the
// families and the other records with their changes, and the counts.
func printDiff(r diff.Result, w io.Writer) {
	if len(r.Header) > 0 {
		fmt.Fprintln(w, "Header")
		for _, d := range r.Header {
			fmt.Fprintf(w, "  %s %s\n", d.Kind.Symbol(), d)
		}
	}
	for _, section := range []struct {
		name string
		recs []diff.Record
	}{{"People", r.People}, {"Families", r.Families}, {"Other records", r.Others}} {
		if len(section.recs) == 0 {
			continue
		}
		fmt.Fprintln(w, section.name)
		idW := 0
		for _, rec := range section.recs {
			idW = max(idW, len(rec.ID))
		}
		for _, rec := range section.recs {
			title := rec.Title
			if rec.Tag != "INDI" && rec.Tag != "FAM" {
				title = strings.TrimSpace(rec.Noun() + " " + title)
			}
			fmt.Fprintf(w, "  %s %-*s  %s\n", rec.Kind.Symbol(), idW, rec.ID, title)
			for _, d := range rec.Details {
				fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", idW+6), d)
			}
		}
	}
	fmt.Fprintln(w, r.Summary())
}

func printStats(doc *gedcom.Document, name string, w io.Writer) {
	s := genealogy.Compute(doc)
	fmt.Fprintf(w, "File:          %s (%s", name, doc.Encoding)
	if doc.Version != "" {
		fmt.Fprintf(w, ", GEDCOM %s", doc.Version)
	}
	fmt.Fprintln(w, ")")
	fmt.Fprintf(w, "Individuals:   %d (%d male, %d female, %d other/unknown)\n", s.Individuals, s.Males, s.Females, s.OtherSex)
	fmt.Fprintf(w, "Families:      %d\n", s.Families)
	fmt.Fprintf(w, "Events:        %d\n", s.Events)
	fmt.Fprintf(w, "Places:        %d\n", s.Places)
	fmt.Fprintf(w, "Sources:       %d\n", s.Sources)
	if s.EarliestYear != 0 {
		fmt.Fprintf(w, "Years:         %d–%d\n", s.EarliestYear, s.LatestYear)
	}
	fmt.Fprintf(w, "Generations:   %d\n", s.Generations)
	if s.Lifespans > 0 {
		fmt.Fprintf(w, "Avg lifespan:  %.1f years (%d people)\n", s.AverageLifespan, s.Lifespans)
	}
	if len(s.Surnames) > 0 {
		var top []string
		for _, nc := range s.Surnames[:min(10, len(s.Surnames))] {
			top = append(top, fmt.Sprintf("%s (%d)", nc.Name, nc.Count))
		}
		fmt.Fprintf(w, "Top surnames:  %s\n", strings.Join(top, ", "))
	}
	if s.AncestorPositions > 0 {
		fmt.Fprintf(w, "Collapse:      %.1f%% of ancestor positions repeat an ancestor (%s)\n",
			s.PedigreeCollapse(), s.CollapsedPeople())
	}
	if doc.Redactions > 0 {
		fmt.Fprintf(w, "Restricted:    %d hidden (confidential or private; see -show-restricted)\n", doc.Redactions)
	}
	fmt.Fprintf(w, "Warnings:      %d\n", len(doc.Warnings))
	for _, warn := range doc.Warnings {
		fmt.Fprintf(w, "  %s\n", warn)
	}
}
