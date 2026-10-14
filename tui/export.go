package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/export"
	"github.com/efi/goged/gedcom"
)

// The rows of the export view: settings, then the action. Further
// settings, such as a temporal cutoff, become further rows.
const (
	exportRowFormat = iota
	exportRowPeople
	exportRowFile
	exportRowExport
	exportRows
)

// exportCount is the size of a selection.
type exportCount struct{ people, links int }

type exportState struct {
	row     int
	format  int // index into export.Formats
	scope   export.Scope
	path    textinput.Model
	editing bool
	before  string // the path when editing started, restored by esc
	edited  bool   // the path was typed and no longer follows the settings
	confirm string // the existing file that pressing enter again replaces

	// The sizes of the selections, worked out when the view is shown: all
	// people, and those connected to componentOf.
	all         *exportCount
	component   exportCount
	componentOf *gedcom.Individual
}

func newExportState() exportState {
	return exportState{path: newTextInput("", "file name")}
}

// ensureExport works out the sizes of the selections and the default file
// name; it runs when the view is shown and when the marked person changes.
func (m *Model) ensureExport() {
	e := &m.export
	if e.all == nil {
		sel, _ := export.Select(m.doc, export.Options{})
		e.all = &exportCount{len(sel.People), len(sel.Links)}
	}
	if m.reference != e.componentOf {
		e.componentOf = m.reference
		e.component = exportCount{}
		if m.reference != nil {
			sel, _ := export.Select(m.doc, export.Options{Scope: export.Component, Root: m.reference})
			e.component = exportCount{len(sel.People), len(sel.Links)}
		}
	}
	if m.reference == nil && e.scope == export.Component {
		e.scope = export.All
	}
	if !e.edited {
		e.path.SetValue(m.defaultExportPath())
	}
}

// defaultExportPath names the export after the file and the people in it:
// family.gexf, or family-I1.gexf for those connected to I1.
func (m *Model) defaultExportPath() string {
	base := strings.TrimSuffix(m.opts.Title, filepath.Ext(m.opts.Title))
	if base == "" || m.opts.Title == "stdin" {
		base = "goged"
	}
	if m.export.scope == export.Component && m.reference != nil {
		base += "-" + strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
				return r
			}
			return '_'
		}, m.reference.ID)
	}
	return base + m.exportFormats()[m.export.format].Extension
}

func (m *Model) exportOptions() export.Options {
	today := m.opts.Today
	if today.IsZero() {
		today = time.Now()
	}
	return export.Options{Scope: m.export.scope, Root: m.reference, Title: m.opts.Title, Generated: today}
}

// changeExportSetting moves the setting of the selected row by dir.
func (m *Model) changeExportSetting(dir int) {
	e := &m.export
	switch e.row {
	case exportRowFormat:
		n := len(m.exportFormats())
		e.format = (e.format + dir + n) % n
	case exportRowPeople:
		if e.scope == export.All && m.reference == nil {
			m.setError("mark a person with m first: then the people connected to them can be exported")
			return
		}
		e.scope = 1 - e.scope
	default:
		return
	}
	if !e.edited {
		e.path.SetValue(m.defaultExportPath())
	}
}

func (m *Model) updateExport(msg tea.KeyMsg) (bool, tea.Cmd) {
	e := &m.export
	confirm := e.confirm
	e.confirm = ""
	switch msg.String() {
	case "up", "k":
		e.row = max(0, e.row-1)
	case "down", "j":
		e.row = min(exportRows-1, e.row+1)
	case "left", "h":
		m.changeExportSetting(-1)
	case "right", "l":
		m.changeExportSetting(1)
	case "enter", " ":
		switch e.row {
		case exportRowFile:
			e.editing, e.before = true, e.path.Value()
			e.path.CursorEnd()
			return true, e.path.Focus()
		case exportRowExport:
			m.runExport(confirm)
		default:
			m.changeExportSetting(1)
		}
	default:
		return false, nil
	}
	return true, nil
}

func (m *Model) updateExportPath(msg tea.KeyMsg) (bool, tea.Cmd) {
	e := &m.export
	switch msg.String() {
	case "enter":
		e.editing = false
		e.path.Blur()
		// An emptied name follows the settings again.
		if strings.TrimSpace(e.path.Value()) == "" {
			e.edited = false
			e.path.SetValue(m.defaultExportPath())
		} else {
			e.edited = e.path.Value() != m.defaultExportPath()
		}
	case "esc":
		e.editing = false
		e.path.Blur()
		e.path.SetValue(e.before)
	default:
		if isViewKey(msg.String()) {
			e.editing = false
			e.path.Blur()
			e.edited = e.path.Value() != m.defaultExportPath()
			return false, nil
		}
		var cmd tea.Cmd
		e.path, cmd = e.path.Update(msg)
		return true, cmd
	}
	return true, nil
}

// runExport writes the file. An existing file is only replaced if the
// previous key already asked to (confirm).
func (m *Model) runExport(confirm string) {
	e := &m.export
	opts := m.exportOptions()
	sel, err := export.Select(m.doc, opts)
	if errors.Is(err, export.ErrNoRoot) {
		m.setError("mark a person with m first: then the people connected to them can be exported")
		return
	}
	path := strings.TrimSpace(e.path.Value())
	if _, err := os.Stat(path); err == nil && confirm != path {
		e.confirm = path
		m.setError(path + " exists: press enter again to replace it")
		return
	}
	f := m.exportFormats()[e.format]
	if err := export.WriteFile(path, f, sel, opts); err != nil {
		m.setError("export failed: " + err.Error())
		return
	}
	// The file name first: a long directory may not fit on the line.
	where := filepath.Base(path)
	if abs, err := filepath.Abs(path); err == nil {
		where += " in " + filepath.Dir(abs)
	}
	m.setStatus(fmt.Sprintf("exported %s and %s as %s to %s", plural(len(sel.People), "person", "people"),
		plural(len(sel.Links), "relationship", "relationships"), f.Name, where))
}

// exportNotes explain what each format contains.
var exportNotes = map[string][]string{
	"GEXF": {
		"People become nodes with their sex, birth and death. Parents point to their",
		"children; partners are linked both ways.",
	},
	"PDF": {
		"Contents with indexes, then a page per person in name order and the places",
		"with their events. Page numbers link to the pages. Nobody is marked.",
	},
	"HTML": {
		"One page for the browser: contents, a section per person in name order and the",
		"places with their events, linked to each other. Nobody is marked.",
	},
}

func (m Model) viewExport(h int) string {
	e := &m.export
	formats := (&m).exportFormats()
	f := formats[e.format]
	var out []string
	out = append(out, m.st.section.Render("Export")+m.st.dim.Render(" · ↑↓ choose · ←→ change · enter edits the file name or exports"), "")

	row := func(r int, label, value, note string) {
		text := pad(label, 12) + value
		if r == e.row {
			text = m.st.selected.Render("▸ " + text)
		} else {
			text = "  " + m.st.label.Render(pad(label, 12)) + value
		}
		if note != "" {
			text += "  " + m.st.dim.Render(note)
		}
		out = append(out, fit(text, m.width))
	}
	choice := func(s string, n int) string {
		if n > 1 {
			return "◂ " + s + " ▸"
		}
		return s
	}

	row(exportRowFormat, "Format", choice(f.Name, len(formats)), f.Description)

	people, count := "all", e.all
	if e.scope == export.Component && m.reference != nil {
		people, count = "connected to ★ "+m.reference.DisplayName(), &e.component
	}
	note := ""
	if count != nil {
		note = plural(count.people, "person", "people") + " · " + plural(count.links, "relationship", "relationships")
	}
	row(exportRowPeople, "People", choice(people, 2), note)
	if m.reference == nil {
		out = append(out, fit(strings.Repeat(" ", 14)+m.st.dim.Render("mark a person with m to export only the people connected to them"), m.width))
	}

	if e.editing {
		out = append(out, fit("  "+m.st.label.Render(pad("File", 12))+e.path.View(), m.width))
	} else {
		row(exportRowFile, "File", e.path.Value(), "")
	}
	out = append(out, "")
	row(exportRowExport, "", "[ Export ]", "")

	out = append(out, "")
	for _, l := range exportNotes[f.Name] {
		out = append(out, m.st.dim.Render("  "+l))
	}
	if n := m.doc.Redactions; n > 0 {
		out = append(out, m.st.dim.Render(fmt.Sprintf("  %d restricted records are hidden and not exported (see -show-restricted).", n)))
	}
	for i := range out {
		out[i] = fit(out[i], m.width)
	}
	if len(out) > h {
		out = out[:h]
	}
	return strings.Join(out, "\n")
}
