package tui

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Models of one document share what is worked out from it, and keep their
// own state.
func TestSharedModels(t *testing.T) {
	sh := NewShared(loadSample(t))
	a := &app{t: t, m: NewModel(sh, Options{Title: "family.ged"})}
	b := &app{t: t, m: NewModel(sh, Options{Title: "family.ged"})}
	a.resize(100, 30)
	b.resize(120, 40)
	for _, x := range []*app{a, b} {
		x.press("alt+4", "alt+5", "alt+6", "s", "alt+7")
	}
	switch {
	case a.m.index != b.m.index || a.m.index != sh.index:
		t.Error("the search index is not shared")
	case a.m.events != b.m.events || a.m.events == nil:
		t.Error("the event index is not shared")
	case a.m.places.root != b.m.places.root || a.m.places.root == nil:
		t.Error("the places are not shared")
	case &a.m.mapv.places[0] != &b.m.mapv.places[0]:
		t.Error("the places on the map are not shared")
	case &a.m.mapv.sn.names[0] != &b.m.mapv.sn.names[0]:
		t.Error("the surnames are not shared")
	}

	// Each has its own marked person, view and filter.
	a.openPerson("I3")
	a.press("m", "4", "f")
	a.typ("type:marr")
	a.press("enter")
	b.openPerson("I14")
	if b.m.reference != nil || b.m.ev.input.Value() != "" || b.m.active != viewPerson {
		t.Errorf("b: marked %v, filter %q, view %d", b.m.reference, b.m.ev.input.Value(), b.m.active)
	}
	if a.m.reference == nil || a.m.reference.ID != "I3" || a.m.ev.input.Value() != "type:marr" {
		t.Errorf("a: marked %v, filter %q", a.m.reference, a.m.ev.input.Value())
	}
}

// Many models of one document can be used at once; run with -race.
func TestSharedModelsConcurrently(t *testing.T) {
	sh := NewShared(loadSample(t))
	tours := [][]string{
		{"/", "s", "m", "i", "t", "enter", "down", "space", "d", "h", "s", "6", "M", "n", "l", "c", "esc", "s", "n", "7", "8", "down", "enter", "C", "esc", "alt+G", "9", "4", "f", "x", "enter", "M"},
		{"/", "j", "o", "h", "n", "enter", "c", "M", "n", "N", "5", "enter", "down", "M", "7", "down", "enter", "3", "t", "+", "-", "8", "space", "4", "a", "enter", "m", "C", "esc", "alt+G", "2", "G", "g"},
		{"8", "down", "down", "down", "enter", "down", "o", "C", "esc", "M", "6", "s", "n", "N", "+", "-", "left", "right", "7", "end", "home", "5", "f", "l", "enter", "enter", "M", "2", "W", "c"},
	}
	var wg sync.WaitGroup
	for i := range 3 * len(tours) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := NewModel(sh, Options{Title: "family.ged"})
			send := func(msg tea.Msg) {
				model, _ := m.Update(msg)
				m = model.(Model)
				_ = m.View()
			}
			send(tea.WindowSizeMsg{Width: 90 + 10*(i%3), Height: 30})
			for _, k := range tours[i%len(tours)] {
				send(keyMsg(k))
			}
		}()
	}
	wg.Wait()
}
