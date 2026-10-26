package tui

import (
	"fmt"
	"strings"
	"testing"
)

func TestHistorySteps(t *testing.T) {
	var h history
	if text, ok := h.step(-1, ""); ok || text != "" {
		t.Errorf("an empty history steps to %q", text)
	}
	h.add("a")
	h.add("  ") // blank: not kept
	h.add("b")
	h.add("a") // moves to the end
	if fmt.Sprint(h.entries) != "[b a]" {
		t.Fatalf("entries %q", h.entries)
	}

	// Back from what was typed: it is kept as the newest entry.
	steps := []struct {
		dir        int
		text, want string
		ok         bool
	}{
		{-1, "c", "a", true},
		{-1, "a", "b", true},
		{-1, "b", "b", false}, // the oldest
		{1, "b", "a", true},
		{1, "a", "c", true},
		{1, "c", "", true}, // past the newest: a new empty line
		{1, "", "", false},
		{-1, "", "c", true},
		{-1, "c", "a", true},
		{1, "a!", "", true}, // changed: kept, then a new empty line
	}
	for i, s := range steps {
		got, ok := h.step(s.dir, s.text)
		if got != s.want || ok != s.ok {
			t.Errorf("step %d (%+d from %q) = %q, %v; want %q, %v", i, s.dir, s.text, got, ok, s.want, s.ok)
		}
	}
	if fmt.Sprint(h.entries) != "[b a c a!]" {
		t.Errorf("entries %q", h.entries)
	}

	for i := range maxHistory + 5 {
		h.add(fmt.Sprint(i))
	}
	if len(h.entries) != maxHistory || h.entries[0] != "5" {
		t.Errorf("%d entries from %q", len(h.entries), h.entries[0])
	}
}

func TestSearchHistory(t *testing.T) {
	a := newApp(t, Options{})
	a.typ("smith")
	a.press("enter") // opens a person: the query is used
	a.press("/", "esc")
	if a.m.search.input.Value() != "" {
		t.Fatalf("query %q", a.m.search.input.Value())
	}
	a.typ("jones")
	a.press("alt+up")
	if q := a.m.search.input.Value(); q != "smith" || !strings.Contains(a.view(), "Search: smith") {
		t.Errorf("alt+↑ shows %q", q)
	}
	if want, _ := a.m.index.SearchString("smith"); len(a.m.search.results) != len(want) {
		t.Errorf("%d results, not the %d of smith", len(a.m.search.results), len(want))
	}
	a.press("alt+down")
	if q := a.m.search.input.Value(); q != "jones" {
		t.Errorf("alt+↓ shows %q, want the query typed", q)
	}
	a.press("alt+down")
	if q := a.m.search.input.Value(); q != "" {
		t.Errorf("alt+↓ past the newest shows %q", q)
	}
	a.contains("alt+↑↓ history  tab/alt+1-9 views  ? help")

	// A reload keeps the history.
	a.m.replaceDoc(NewShared(loadSample(t)))
	a.press("alt+up")
	if q := a.m.search.input.Value(); q != "jones" {
		t.Errorf("after a reload, alt+↑ shows %q", q)
	}
}

func TestFilterHistory(t *testing.T) {
	a := newApp(t, Options{})
	a.press("alt+4", "f")
	a.typ("type:birth")
	a.press("enter", "f", "esc", "f")
	a.typ("1850")
	a.press("alt+up")
	if f := a.m.ev.input.Value(); f != "type:birth" || len(a.m.ev.list) != 24 {
		t.Errorf("events: alt+↑ shows %q with %d events", f, len(a.m.ev.list))
	}
	a.contains("esc clear  alt+↑↓ history")
	a.press("alt+down", "alt+down")
	if f := a.m.ev.input.Value(); f != "" {
		t.Errorf("events: past the newest %q", f)
	}
	a.press("alt+up")
	if f := a.m.ev.input.Value(); f != "1850" {
		t.Errorf("events: the filter typed is %q", f)
	}

	// Cleared in the list, a filter is kept too.
	a.press("alt+5", "f")
	a.typ("leeds")
	a.press("enter", "x", "f", "alt+up")
	if f := a.m.places.input.Value(); f != "leeds" || len(a.m.places.rows) == 0 {
		t.Errorf("places: alt+↑ shows %q with %d rows", f, len(a.m.places.rows))
	}
}
