package theme

import "charm.land/lipgloss/v2"

type Styles struct {
	Palette         Palette
	Spinner         lipgloss.Style
	Header          lipgloss.Style
	HeaderBold      lipgloss.Style
	HeaderLabel     lipgloss.Style
	HeaderSecondary lipgloss.Style
	ChartPanel      lipgloss.Style
	ChartAxis       lipgloss.Style
	ChartBorder     lipgloss.Style
	Compute         lipgloss.Style
	Memory          lipgloss.Style
	NVLink          lipgloss.Style
	PCIe            lipgloss.Style
	ComputeCeiling  lipgloss.Style
	DotOffline      string
	DotOnline       string
}

func NewStyles(dark bool) Styles {
	palette := NewPalette(dark)

	borderFg := lipgloss.Color("15")
	axisFg := lipgloss.Color("15")
	computeFg := lipgloss.Color("#00D7FF")
	memoryFg := lipgloss.Color("#FF5F00")

	header := lipgloss.NewStyle().
		Background(palette.Surface).
		Foreground(palette.Text)
	panel := lipgloss.NewStyle().
		Background(palette.Panel).
		Foreground(palette.Text)

	return Styles{
		Palette:         palette,
		Spinner:         lipgloss.NewStyle().Foreground(palette.Negative),
		Header:          header,
		HeaderBold:      lipgloss.NewStyle().Inherit(header).Bold(true),
		HeaderLabel:     lipgloss.NewStyle().Inherit(header),
		HeaderSecondary: lipgloss.NewStyle().Inherit(header).Foreground(palette.TextMuted),
		ChartPanel:      panel,
		ChartAxis:       lipgloss.NewStyle().Inherit(panel).Foreground(axisFg),
		ChartBorder: lipgloss.NewStyle().Inherit(panel).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderFg).
			BorderBackground(palette.Panel),
		Compute:        lipgloss.NewStyle().Foreground(computeFg),
		Memory:         lipgloss.NewStyle().Foreground(memoryFg),
		NVLink:         lipgloss.NewStyle().Foreground(palette.NVLink),
		PCIe:           lipgloss.NewStyle().Foreground(palette.PCIe),
		ComputeCeiling: lipgloss.NewStyle().Foreground(palette.ComputeCeiling),
		DotOffline:     lipgloss.NewStyle().Inherit(header).Foreground(palette.Negative).Render("●"),
		DotOnline:      lipgloss.NewStyle().Inherit(header).Foreground(palette.Positive).Render("●"),
	}
}
