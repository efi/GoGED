package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
)

// watchedApp opens a copy of family.ged that the test can change.
func watchedApp(t *testing.T) (*app, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "family.ged")
	data, err := os.ReadFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	load := func() (*gedcom.Document, error) { return gedcom.ParseFile(path) }
	doc, err := load()
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc, Options{Title: "family.ged", Path: path, Load: load})}
	a.resize(100, 30)
	return a, path
}

var fileVersion time.Time

// rewrite changes the file as another program would: re matches what is
// replaced. Every version is a minute newer than the one before.
func rewrite(t *testing.T, path, re, with string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = regexp.MustCompile(re).ReplaceAll(data, []byte(with))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	// A minute after the version before, or after now.
	next := time.Now().Add(time.Minute)
	if !fileVersion.Before(next) {
		next = fileVersion.Add(time.Minute)
	}
	fileVersion = next
	if err := os.Chtimes(path, fileVersion, fileVersion); err != nil {
		t.Fatal(err)
	}
}

// poll looks at the file, as the watcher does every second. It reports
// whether the watcher goes on.
func (a *app) poll(path string) bool { return a.send(statFile(path)) != nil }

// answerable lets the dialog take keys at once.
func (a *app) answerable() { a.m.watch.since = time.Time{} }

// finish delivers what a command brings, such as the file loaded again.
func (a *app) finish(cmd tea.Cmd) {
	a.t.Helper()
	if cmd == nil {
		a.t.Fatal("no command")
	}
	a.send(cmd())
}

func TestReloadAsks(t *testing.T) {
	a, path := watchedApp(t)
	a.openPerson("I1")
	a.openPerson("I3")
	a.press("m", "alt+4", "f")
	a.typ("type:death")
	a.press("enter", "alt+2")

	if !a.poll(path) || a.m.watch.asking {
		t.Fatal("asked about an unchanged file")
	}
	rewrite(t, path, `NAME John /Smith/`, `NAME Johann /Smith/`)
	// The file may still be being written: the dialog waits until it
	// stays the same.
	a.poll(path)
	a.notContains("Another program")
	a.poll(path)
	a.contains("family.ged changed", "Another program has changed family.ged.",
		"[ Yes, reload ]  [ Not this time ]  [ Never this session ]", "y reload  n not this time")

	// Keys right after the dialog appeared were meant for the view.
	a.press("n")
	if !a.m.watch.asking {
		t.Fatal("a key in the first moment answered the dialog")
	}
	a.answerable()
	a.press("right", "right", "right", "left", "left")
	if a.m.watch.choice != choiceReload {
		t.Errorf("choice %d", a.m.watch.choice)
	}
	cmd := a.press("enter")
	a.contains("loading family.ged again")
	a.finish(cmd)
	a.contains("loaded family.ged again: 24 people · 9 families", "Johann Smith", "★ Johann Smith")
	a.notContains("Another program")
	if a.currentID() != "I3" || a.m.active != viewPerson || a.m.ev.input.Value() != "type:death" {
		t.Errorf("person %s, view %d, filter %q", a.currentID(), a.m.active, a.m.ev.input.Value())
	}
	a.press("b")
	if a.currentID() != "I1" {
		t.Errorf("history: %s", a.currentID())
	}

	// Not this time: asked again at the next change only.
	rewrite(t, path, `Johann`, `Johannes`)
	a.poll(path)
	a.poll(path)
	a.answerable()
	a.press("n")
	a.contains("family.ged was not loaded again")
	a.poll(path)
	a.poll(path)
	if a.m.watch.asking {
		t.Error("asked again about the same version")
	}
	rewrite(t, path, `Johannes`, `Jo`)
	a.poll(path)
	a.poll(path)
	if !a.m.watch.asking {
		t.Fatal("not asked about a further change")
	}

	// Never this session, by mouse; the watcher stops.
	a.wheel(tea.MouseButtonWheelDown, 50, 15)
	a.clickOn("Another program")
	a.clickOn("2 Person")
	if !a.m.watch.asking || a.m.active != viewPerson {
		t.Fatal("only the buttons answer")
	}
	x, y := a.find("[ Never this session ]")
	a.click(x+3, y)
	a.contains("family.ged is not watched any more in this session")
	rewrite(t, path, `NAME Jo /`, `NAME John /`)
	for range 2 {
		if a.poll(path) {
			t.Error("the watcher goes on after never")
		}
	}
	if a.m.watch.asking {
		t.Error("asked after never")
	}
	a.contains("Johann Smith")
}

func TestReloadKeepsWhatItCan(t *testing.T) {
	a, path := watchedApp(t)
	a.openPerson("I14")
	a.openPerson("I24")
	a.press("t", "alt+5")
	a.clickOn("▾ Canada")
	a.press("alt+8", "down", "down", "down", "down", "right", "right", "alt+9", "right")
	a.press("alt+6")
	a.clickOn("● Köln")
	a.press("alt+5", "end", "enter")

	// The infant is no longer in the file.
	rewrite(t, path, `(?s)0 @I24@ INDI\n.*?\n(0 )`, "$1")
	rewrite(t, path, `1 CHIL @I24@\n`, "")
	a.poll(path)
	a.poll(path)
	a.answerable()
	a.finish(a.press("y"))
	a.contains("loaded family.ged again: 23 people")
	if a.currentID() != "I14" || len(a.m.history) != 1 {
		t.Errorf("person %s, history %d", a.currentID(), len(a.m.history))
	}
	// The places keep what was folded and the events shown.
	if p := a.m.places; p.detail == nil || p.detail.Name != "Köln" || a.m.active != viewPlaces {
		t.Errorf("view %d, detail %v", a.m.active, p.detail)
	}
	a.press("esc")
	a.contains("▹ Canada")
	// The checks keep their thresholds, the export its format, the map its
	// selection, the tree its chart.
	if c := a.m.checks; c.params[4] != 112 || c.rows[c.cursor].check != 4 || a.m.export.format != 1 {
		t.Errorf("checks %v, cursor %d, export format %d", c.params, c.cursor, a.m.export.format)
	}
	if s := a.m.mapv; s.places[s.sel].Name != "Köln" {
		t.Errorf("map selection %s", s.places[s.sel].Name)
	}
	if a.m.tree.mode != modePedigree || a.m.tree.chart.Nodes[0].Ind.ID != "I14" {
		t.Error("tree")
	}

	// Nobody left in the history: the search is shown.
	b, path := watchedApp(t)
	b.openPerson("I24")
	rewrite(t, path, `(?s)0 @I24@ INDI\n.*?\n(0 )`, "$1")
	rewrite(t, path, `1 CHIL @I24@\n`, "")
	b.poll(path)
	b.poll(path)
	b.answerable()
	b.finish(b.press("y"))
	if b.m.active != viewSearch || b.currentID() != "" {
		t.Errorf("view %d, person %s", b.m.active, b.currentID())
	}
	b.contains("23 people")
}

func TestReloadFails(t *testing.T) {
	a, path := watchedApp(t)
	rewrite(t, path, `John`, `Jon`)
	a.poll(path)
	a.poll(path)
	a.answerable()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	a.finish(a.press("y"))
	a.contains("family.ged could not be loaded again")
	// A missing file is not asked about; when it is back, it is.
	if !a.poll(path) || a.m.watch.asking {
		t.Error("asked about a missing file")
	}
	if err := os.WriteFile(path, []byte("0 HEAD\n0 TRLR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.poll(path)
	a.poll(path)
	if !a.m.watch.asking {
		t.Error("not asked when the file was back")
	}
}

func TestOverlay(t *testing.T) {
	r := ansi.ResetStyle
	if got := overlay("abcdefghij\nklmnopqrst", []string{"XY"}, 3, 1); got != "abcdefghij\nklm"+r+"XY"+r+"pqrst" {
		t.Errorf("%q", got)
	}
	if got := overlay("ab\ncd", []string{"XY", "Z", "lost"}, 4, 0); got != "ab  "+r+"XY"+r+"\ncd  "+r+"Z"+r {
		t.Errorf("%q", got)
	}
	red := "\x1b[31m"
	// The text on either side keeps its style.
	if got := overlay(red+"abcdef"+r, []string{"X"}, 2, 0); got != red+"ab"+r+r+"X"+r+red+"def"+r {
		t.Errorf("styled: %q", got)
	}
}
