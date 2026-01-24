package styles

import "github.com/charmbracelet/lipgloss"

var (
	PrimaryColor   = lipgloss.Color("#7C3AED")
	SecondaryColor = lipgloss.Color("#06B6D4")
	SuccessColor   = lipgloss.Color("#22C55E")
	WarningColor   = lipgloss.Color("#EAB308")
	ErrorColor     = lipgloss.Color("#EF4444")
	MutedColor     = lipgloss.Color("#6B7280")
	SurfaceColor   = lipgloss.Color("#1F2937")
	TextColor      = lipgloss.Color("#F9FAFB")

	Muted = lipgloss.NewStyle().Foreground(MutedColor)

	TabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Padding(0, 2)

	TabInactive = lipgloss.NewStyle().
			Foreground(MutedColor).
			Padding(0, 2)

	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(PrimaryColor).
		Padding(0, 1)

	StatusBar = lipgloss.NewStyle().
			Foreground(MutedColor).
			Padding(0, 1)

	CardBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(PrimaryColor).
			Padding(0, 1)

	SiteCardBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SecondaryColor).
			Padding(0, 1)

	StatValue = lipgloss.NewStyle().
			Bold(true).
			Foreground(TextColor)

	StatLabel = lipgloss.NewStyle().
			Foreground(MutedColor)

	StatusSuccess = lipgloss.NewStyle().Foreground(SuccessColor)
	StatusError   = lipgloss.NewStyle().Foreground(ErrorColor)
	StatusPending = lipgloss.NewStyle().Foreground(WarningColor)

	TableHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Padding(0, 1)

	TableCell = lipgloss.NewStyle().
			Padding(0, 1)

	TableSelected = lipgloss.NewStyle().
			Background(PrimaryColor).
			Foreground(TextColor)

	Notification = lipgloss.NewStyle().
			Foreground(SuccessColor).
			Padding(0, 1)
)
