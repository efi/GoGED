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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/licenses"
	"github.com/efi/goged/search"
	"github.com/efi/goged/tui"
	"github.com/efi/goged/worldmap"
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
	if fromStdin {
		// The file came through stdin, so read keys from the terminal.
		progOpts = append(progOpts, tea.WithInputTTY())
	}
	_, err := tea.NewProgram(tui.New(doc, opts), progOpts...).Run()
	return err
}

const usageText = `goged — browse GEDCOM genealogy files in the terminal

Usage:
  goged [flags] FILE.ged        open the interactive browser
  goged -q QUERY FILE.ged       print matching people and exit

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
		timeline = fs.String("timeline", "", "print the timeline of the person with `ID`")
		relate   = fs.String("relate", "", "print how two people are related, e.g. -relate I1,I20 (`ID1,ID2`)")
		events   = fs.String("events", "", "print events matching `FILTER`, e.g. \"type:marr 1850..1870\"")
		allEv    = fs.Bool("all", false, "with -events: include all events, not only births, marriages and deaths")
		places   = fs.Bool("places", false, "print all places as a hierarchy with the number of events and people")
		stats    = fs.Bool("stats", false, "print statistics and warnings about the file")
		gens     = fs.Int("gen", 4, "number of `generations` shown in trees")
		ascii    = fs.Bool("ascii", false, "draw trees with ASCII characters only")
		person   = fs.String("person", "", "open the browser at the person with `ID`")
		showRes  = fs.Bool("show-restricted", false, "show data marked confidential or private (RESN), which is hidden by default")
		showVer  = fs.Bool("version", false, "print the version and exit")
		showLic  = fs.Bool("licenses", false, "print the licenses of the map data and the third-party software")
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
		fmt.Fprintf(stdout, "%s\n\n%s", worldmap.License, licenses.ThirdParty)
		return 0
	}
	if len(positional) != 1 {
		fs.Usage()
		return 2
	}

	path := positional[0]
	var (
		doc *gedcom.Document
		err error
	)
	if path == "-" {
		doc, err = gedcom.Parse(stdin)
	} else {
		doc, err = gedcom.ParseFile(path)
	}
	if err != nil {
		fmt.Fprintf(stderr, "goged: %s: %v\n", path, err)
		return 1
	}
	if !*showRes {
		doc = doc.Redacted()
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

	switch {
	case set["q"]:
		return printQuery(doc, *query, stdout, stderr)
	case set["tree"]:
		ind, ok := lookup(*pedigree)
		if !ok {
			return 1
		}
		fmt.Fprint(stdout, chart.Pedigree(ind, chartOpts).String())
		return 0
	case set["desc"]:
		ind, ok := lookup(*desc)
		if !ok {
			return 1
		}
		fmt.Fprint(stdout, chart.Descendants(ind, chartOpts).String())
		return 0
	case set["timeline"]:
		ind, ok := lookup(*timeline)
		if !ok {
			return 1
		}
		printTimeline(ind, stdout)
		return 0
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
		printRelationship(indA, indB, stdout)
		return 0
	case set["events"]:
		return printEvents(doc, *events, *allEv, stdout, stderr)
	case *places:
		printPlaces(doc, stdout)
		return 0
	case *stats:
		printStats(doc, filepath.Base(path), stdout)
		return 0
	}

	opts := tui.Options{Title: filepath.Base(path), StartPerson: *person, Generations: *gens, ASCII: *ascii}
	if path == "-" {
		opts.Title = "stdin"
	}
	if err := startTUI(doc, opts, path == "-"); err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 1
	}
	return 0
}

func printQuery(doc *gedcom.Document, q string, stdout, stderr io.Writer) int {
	results, err := search.NewIndex(doc).SearchString(q)
	if err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 2
	}
	for _, r := range results {
		ind := r.Individual
		place := ""
		if b := ind.FirstDatedEvent(gedcom.BirthTags...); b != nil {
			place = b.Place.String()
		}
		fmt.Fprintln(stdout, strings.TrimRight(fmt.Sprintf("%-6s %-32s %-12s %s", ind.ID, ind.SortName(), ind.Lifespan(), place), " "))
	}
	return 0
}

func printTimeline(ind *gedcom.Individual, w io.Writer) {
	fmt.Fprintf(w, "%s (%s)\n", ind.DisplayName(), ind.ID)
	for _, e := range genealogy.Timeline(ind, genealogy.TimelineOptions{Relatives: true}) {
		date := "—"
		if e.Event.Date.IsValid() {
			date = e.Event.Date.String()
		}
		title := e.Title()
		if e.Own() && e.Spouse != nil {
			title += " with " + e.Spouse.DisplayName()
		}
		var rest []string
		if d := e.Event.Detail(); d != "" {
			rest = append(rest, d)
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
		lineText := fmt.Sprintf("  %-22s %s", date, title)
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

func printEvents(doc *gedcom.Document, filter string, all bool, stdout, stderr io.Writer) int {
	q, err := search.ParseEventQuery(filter)
	if err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 2
	}
	for _, ev := range search.NewEventIndex(doc.Events()).Filter(q, !all) {
		date := "—"
		if ev.Date.IsValid() {
			date = ev.Date.String()
		}
		var names []string
		for _, p := range ev.Principals() {
			names = append(names, p.DisplayName())
		}
		label := ev.Label()
		if d := ev.Detail(); d != "" && ev.Class() == gedcom.ClassAttribute {
			label += ": " + d
		}
		fmt.Fprintln(stdout, strings.TrimRight(fmt.Sprintf("%-20s %-18s %-36s %s", date, label, strings.Join(names, " & "), ev.Place.String()), " "))
	}
	return 0
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
			line := fmt.Sprintf("%-40s %5d %-6s %5d %s", name, n.Count(), word(n.Count(), "event", "events"), n.People(), word(n.People(), "person", "people"))
			fmt.Fprintln(w, line)
		}
		return true
	})
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
	if doc.Redactions > 0 {
		fmt.Fprintf(w, "Restricted:    %d hidden (confidential or private; see -show-restricted)\n", doc.Redactions)
	}
	fmt.Fprintf(w, "Warnings:      %d\n", len(doc.Warnings))
	for _, warn := range doc.Warnings {
		fmt.Fprintf(w, "  %s\n", warn)
	}
}
