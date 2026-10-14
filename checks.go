package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

// now is the current time; tests fix it.
var now = time.Now

// chosenCheck is a check to run in batch mode, with its threshold.
type chosenCheck struct {
	check genealogy.Check
	param int
}

// chooseChecks reads the value of -checks: all, checks, hints, or IDs of
// checks separated by commas, each with a threshold after = if wanted:
// old-age=100,similar-places=somewhat-resembling.
func chooseChecks(spec string) ([]chosenCheck, error) {
	var out []chosenCheck
	add := func(hints, checks bool) {
		for _, c := range genealogy.Checks {
			if (c.Hint && hints) || (!c.Hint && checks) {
				out = append(out, chosenCheck{c, c.Param})
			}
		}
	}
	switch strings.TrimSpace(spec) {
	case "all":
		add(true, true)
		return out, nil
	case "checks":
		add(false, true)
		return out, nil
	case "hints":
		add(true, false)
		return out, nil
	}
	var ids []string
	for _, c := range genealogy.Checks {
		ids = append(ids, c.ID)
	}
	for _, part := range strings.Split(spec, ",") {
		id, value, withValue := strings.Cut(strings.TrimSpace(part), "=")
		if id == "" {
			continue
		}
		var c *genealogy.Check
		for i := range genealogy.Checks {
			if genealogy.Checks[i].ID == id {
				c = &genealogy.Checks[i]
			}
		}
		if c == nil {
			return nil, fmt.Errorf("unknown check %q (all, checks, hints or %s)", id, strings.Join(ids, ", "))
		}
		param := c.Param
		if withValue {
			var err error
			if param, err = threshold(*c, value); err != nil {
				return nil, err
			}
		}
		out = append(out, chosenCheck{*c, param})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no check chosen (all, checks, hints or %s)", strings.Join(ids, ", "))
	}
	return out, nil
}

// threshold reads the threshold of a check: a number, or one of its
// levels, with - or spaces between the words.
func threshold(c genealogy.Check, value string) (int, error) {
	if !c.HasParam() {
		return 0, fmt.Errorf("%s has no threshold", c.ID)
	}
	if c.Levels != nil {
		var names []string
		for i, l := range c.Levels {
			name := strings.ReplaceAll(l, " ", "-")
			if strings.EqualFold(strings.ReplaceAll(value, " ", "-"), name) {
				return i, nil
			}
			names = append(names, name)
		}
		return 0, fmt.Errorf("%s: expects %s", c.ID, strings.Join(names, ", "))
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < c.Min || n > c.Max {
		return 0, fmt.Errorf("%s: expects a number from %d to %d (%s)", c.ID, c.Min, c.Max, c.Unit)
	}
	return n, nil
}

// checkResult is what a check found.
type checkResult struct {
	chosenCheck
	findings []genealogy.Finding
	summary  string
}

// runChecks prints the findings of the checks chosen by spec, in two
// sections, checks and hints.
func runChecks(doc *gedcom.Document, spec string, marked *gedcom.Individual, asJSON bool, stdout, stderr io.Writer) int {
	chosen, err := chooseChecks(spec)
	if err != nil {
		fmt.Fprintf(stderr, "goged: -checks: %v\n", err)
		return 2
	}
	ctx := genealogy.Context{Marked: marked, Today: now()}
	var results []checkResult
	for _, cc := range chosen {
		results = append(results, checkResult{cc, cc.check.Run(doc, cc.param, ctx), cc.check.Summary(doc, cc.param, ctx)})
	}
	if asJSON {
		if err := writeJSON(stdout, checksJSON(results, ctx)); err != nil {
			fmt.Fprintf(stderr, "goged: %v\n", err)
			return 1
		}
		return 0
	}
	for _, hints := range []bool{false, true} {
		var section []checkResult
		findings, people := 0, map[*gedcom.Individual]bool{}
		for _, r := range results {
			if r.check.Hint != hints {
				continue
			}
			section = append(section, r)
			findings += len(r.findings)
			for _, f := range r.findings {
				if f.Person != nil {
					people[f.Person] = true
				}
			}
		}
		if len(section) == 0 {
			continue
		}
		title := "Checks"
		if hints {
			title = "Hints"
		}
		fmt.Fprintf(stdout, "%s · %d %s in %d %s\n", title, findings, word(findings, "finding", "findings"), len(people), word(len(people), "person", "people"))
		for _, r := range section {
			fmt.Fprintf(stdout, "  %s: %d\n", r.check.Name(r.param), len(r.findings))
			if r.summary != "" {
				fmt.Fprintf(stdout, "      %s\n", r.summary)
			}
			for _, f := range r.findings {
				line := "    "
				if f.Person != nil {
					line += pad(f.Person.ID, 6) + " " + f.Person.DisplayName()
					if ls := f.Person.Lifespan(); ls != "" {
						line += " (" + ls + ")"
					}
					line += ": "
				}
				fmt.Fprintln(stdout, line+f.Message)
			}
		}
	}
	return 0
}

func word(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

type jsonChecks struct {
	Date   string      `json:"date"` // the day the checks were run, as some depend on it
	Marked *jsonRef    `json:"marked,omitempty"`
	Checks []jsonCheck `json:"checks"`
}

type jsonCheck struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Hint      bool          `json:"hint"`
	Threshold any           `json:"threshold,omitempty"` // a number, or a level
	Unit      string        `json:"unit,omitempty"`
	Summary   string        `json:"summary,omitempty"`
	Findings  []jsonFinding `json:"findings"`
}

type jsonFinding struct {
	Person  *jsonRef `json:"person,omitempty"`
	Other   *jsonRef `json:"other,omitempty"`
	Places  []string `json:"places,omitempty"`
	Message string   `json:"message"`
}

func checksJSON(results []checkResult, ctx genealogy.Context) jsonChecks {
	j := jsonChecks{Date: ctx.Today.Format("2006-01-02"), Marked: refJSON(ctx.Marked), Checks: []jsonCheck{}}
	for _, r := range results {
		c := jsonCheck{ID: r.check.ID, Title: r.check.Name(r.param), Hint: r.check.Hint, Unit: r.check.Unit,
			Summary: r.summary, Findings: []jsonFinding{}}
		switch {
		case r.check.Levels != nil:
			c.Threshold = r.check.Levels[r.param]
		case r.check.HasParam():
			c.Threshold = r.param
		}
		for _, f := range r.findings {
			jf := jsonFinding{Person: refJSON(f.Person), Other: refJSON(f.Other), Message: f.Message}
			for _, p := range f.Places {
				jf.Places = append(jf.Places, p.Full)
			}
			c.Findings = append(c.Findings, jf)
		}
		j.Checks = append(j.Checks, c)
	}
	return j
}
