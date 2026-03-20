package oinc

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jasonmadigan/oinc/pkg/tui"
)

var (
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(12)
)

func (s Status) Render() string {
	var b strings.Builder

	b.WriteString(tui.Pig("oinc"))
	b.WriteString("\n")

	row := func(label, value string) {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), value))
	}

	stateVal := s.State
	switch s.State {
	case "running":
		stateVal = tui.Green.Render("● running")
	case "stopped":
		stateVal = tui.Red.Render("● stopped")
	default:
		stateVal = tui.Red.Render("● " + s.State)
	}

	row("Cluster", stateVal)
	row("Runtime", s.Runtime)
	if s.Version != "" {
		row("Version", s.Version)
	}
	if s.Uptime != "" {
		row("Uptime", s.Uptime)
	}

	if s.State != "running" {
		if s.Error != "" {
			b.WriteString("\n")
			row("Error", tui.Red.Render(s.Error))
		}
		return b.String()
	}

	// endpoints box
	var endpoints []string
	if s.APIServer != "" {
		endpoints = append(endpoints, fmt.Sprintf("  %s %s", labelStyle.Render("API"), s.APIServer))
	}
	if s.ConsoleURL != "" {
		endpoints = append(endpoints, fmt.Sprintf("  %s %s", labelStyle.Render("Console"), s.ConsoleURL))
	}
	if s.IngressHTTP != "" || s.IngressHTTPS != "" {
		parts := []string{}
		if s.IngressHTTP != "" {
			parts = append(parts, s.IngressHTTP)
		}
		if s.IngressHTTPS != "" {
			parts = append(parts, s.IngressHTTPS)
		}
		endpoints = append(endpoints, fmt.Sprintf("  %s %s", labelStyle.Render("Ingress"), strings.Join(parts, tui.Dim.Render(" | "))))
	}
	if len(endpoints) > 0 {
		b.WriteString("\n")
		box := tui.Box.Render(strings.Join(endpoints, "\n"))
		b.WriteString(indent(box, 2))
		b.WriteString("\n")
	}

	// addons box
	if len(s.Addons) > 0 {
		addonLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(16)
		var rows []string
		for _, a := range s.Addons {
			dot := tui.StatusDot(a.Ready)
			state := tui.Green.Render("ready")
			if !a.Ready {
				state = tui.Yellow.Render("not ready")
			}
			rows = append(rows, fmt.Sprintf("  %s %s %s", addonLabel.Render(a.Name), dot, state))
		}
		b.WriteString("\n")
		box := tui.Box.Render(strings.Join(rows, "\n"))
		b.WriteString(indent(box, 2))
		b.WriteString("\n")
	}

	if s.Error != "" {
		b.WriteString("\n")
		row("Error", tui.Red.Render(s.Error))
	}

	b.WriteString("\n")
	return b.String()
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = pad + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
