package tui

import "github.com/charmbracelet/lipgloss"

var (
	Pink   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff69b4"))
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308"))
	Dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	Label  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	Bold   = lipgloss.NewStyle().Bold(true)
)

var Box = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#555555")).
	Padding(0, 1)

var BoxTitle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#888888")).
	Bold(true)

func StatusDot(ok bool) string {
	if ok {
		return Green.Render("●")
	}
	return Red.Render("●")
}

func Pig(suffix string) string {
	top := Pink.Render("  ^..^")
	mid := Pink.Render(" ( oo )")
	bot := Pink.Render("  (..)")
	if suffix != "" {
		mid += "  " + suffix
	}
	return top + "\n" + mid + "\n" + bot + "\n"
}
