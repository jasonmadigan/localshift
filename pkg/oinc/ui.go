package oinc

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	pigStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff69b4"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(10)
	greenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
)

// Pig returns the styled pig logo with an optional suffix on the middle line.
func Pig(suffix string) string {
	var b strings.Builder
	b.WriteString(pigStyle.Render("  ^..^") + "\n")
	b.WriteString(pigStyle.Render(" ( oo )"))
	if suffix != "" {
		b.WriteString("  " + suffix)
	}
	b.WriteString("\n")
	b.WriteString(pigStyle.Render("  (..)") + "\n")
	return b.String()
}

func (s Status) Render() string {
	var b strings.Builder

	b.WriteString(Pig("oinc"))
	b.WriteString("\n")

	row := func(label, value string) {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), value))
	}

	stateVal := s.State
	if s.State == "running" {
		stateVal = greenStyle.Render(s.State)
	} else {
		stateVal = redStyle.Render(s.State)
	}

	row("Cluster", stateVal)
	row("Runtime", s.Runtime)
	if s.Version != "" {
		row("Version", s.Version)
	}
	if s.Uptime != "" {
		row("Uptime", s.Uptime)
	}

	if s.State == "running" {
		b.WriteString("\n")
		if s.APIServer != "" {
			row("API", s.APIServer)
		}
		if s.ConsoleURL != "" {
			row("Console", s.ConsoleURL)
		}
		if s.IngressHTTP != "" || s.IngressHTTPS != "" {
			parts := []string{}
			if s.IngressHTTP != "" {
				parts = append(parts, s.IngressHTTP)
			}
			if s.IngressHTTPS != "" {
				parts = append(parts, s.IngressHTTPS)
			}
			row("Ingress", strings.Join(parts, dimStyle.Render(" | ")))
		}
		if len(s.Addons) > 0 {
			addonLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(16)
			b.WriteString("\n")
			for _, a := range s.Addons {
				state := greenStyle.Render("ready")
				if !a.Ready {
					state = yellowStyle.Render("not ready")
				}
				b.WriteString(fmt.Sprintf("  %s %s\n", addonLabel.Render(a.Name), state))
			}
		}
	}

	if s.Error != "" {
		b.WriteString("\n")
		row("Error", redStyle.Render(s.Error))
	}

	return b.String()
}
