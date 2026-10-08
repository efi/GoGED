package tui

import "github.com/charmbracelet/lipgloss"

// styles collects the lipgloss styles used by the interface. Colors adapt
// to light and dark terminals; with NO_COLOR or a non-color terminal the
// layout still works because selection is also marked with a glyph.
type styles struct {
	title       lipgloss.Style
	tabActive   lipgloss.Style
	tabInactive lipgloss.Style
	section     lipgloss.Style
	label       lipgloss.Style
	name        lipgloss.Style
	dim         lipgloss.Style
	selected    lipgloss.Style
	err         lipgloss.Style
	status      lipgloss.Style
	key         lipgloss.Style
	root        lipgloss.Style
	reference   lipgloss.Style
	mapBorder   lipgloss.Style
	mapCoast    lipgloss.Style
	mapWater    lipgloss.Style
	mapMarker   lipgloss.Style
	mapSelected lipgloss.Style
	mapLabel    lipgloss.Style
}

func defaultStyles() styles {
	accent := lipgloss.AdaptiveColor{Light: "#5B3FA6", Dark: "#B9A2FF"}
	subtle := lipgloss.AdaptiveColor{Light: "#7A7A7A", Dark: "#8A8A8A"}
	warm := lipgloss.AdaptiveColor{Light: "#A15C00", Dark: "#FFB454"}
	return styles{
		title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1A1A1A"}).Background(accent).Padding(0, 1),
		tabActive:   lipgloss.NewStyle().Bold(true).Foreground(accent).Underline(true),
		tabInactive: lipgloss.NewStyle().Foreground(subtle),
		section:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		label:       lipgloss.NewStyle().Foreground(subtle),
		name:        lipgloss.NewStyle().Bold(true),
		dim:         lipgloss.NewStyle().Foreground(subtle),
		selected:    lipgloss.NewStyle().Reverse(true).Bold(true),
		err:         lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C0262D", Dark: "#FF6B6B"}).Bold(true),
		status:      lipgloss.NewStyle().Foreground(subtle),
		key:         lipgloss.NewStyle().Bold(true).Foreground(warm),
		root:        lipgloss.NewStyle().Bold(true).Underline(true),
		reference:   lipgloss.NewStyle().Foreground(warm).Bold(true),
		mapBorder:   lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: "#6E6E6E"}),
		mapCoast:    lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4E7A32", Dark: "#9CCB7A"}),
		mapWater:    lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1E6BB8", Dark: "#5AA9E6"}),
		mapMarker:   lipgloss.NewStyle().Foreground(warm).Bold(true),
		mapSelected: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C0262D", Dark: "#FF6B6B"}).Bold(true),
		mapLabel:    lipgloss.NewStyle().Foreground(warm),
	}
}
