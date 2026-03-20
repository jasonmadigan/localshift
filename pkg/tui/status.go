package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusData is the data needed to render the dashboard.
// Provided by the caller to avoid a circular dependency on pkg/oinc.
type StatusData struct {
	State        string
	Runtime      string
	Version      string
	APIServer    string
	ConsoleURL   string
	IngressHTTP  string
	IngressHTTPS string
	Uptime       string
	Addons       []AddonData
	Pods         []PodData
	Error        string
}

type AddonData struct {
	Name  string
	Ready bool
}

type PodData struct {
	Name      string
	Namespace string
	Ready     string
	Status    string
}

// StatusFetchFunc fetches current cluster state.
type StatusFetchFunc func() StatusData

type refreshMsg struct{ data StatusData }

type StatusModel struct {
	data     StatusData
	fetch    StatusFetchFunc
	viewport viewport.Model
	interval time.Duration
	width    int
	height   int
	ready    bool
}

func NewStatusModel(fetch StatusFetchFunc, interval time.Duration) StatusModel {
	return StatusModel{
		fetch:    fetch,
		interval: interval,
	}
}

func (m StatusModel) Init() tea.Cmd {
	return tea.Batch(
		m.doFetch(),
		tea.WindowSize(),
	)
}

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.doFetch()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 16 // approximate header size
		vpHeight := m.height - headerHeight
		if vpHeight < 4 {
			vpHeight = 4
		}
		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent(m.renderPodTable())
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
			m.viewport.SetContent(m.renderPodTable())
		}

	case refreshMsg:
		m.data = msg.data
		if m.ready {
			m.viewport.SetContent(m.renderPodTable())
		}
		cmds = append(cmds, m.scheduleRefresh())
	}

	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m StatusModel) View() string {
	var b strings.Builder

	b.WriteString(Pig("oinc"))
	b.WriteString("\n")

	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(12)
	row := func(l, v string) {
		b.WriteString(fmt.Sprintf("  %s %s\n", label.Render(l), v))
	}

	stateVal := m.data.State
	switch m.data.State {
	case "running":
		stateVal = Green.Render("● running")
	case "stopped":
		stateVal = Red.Render("● stopped")
	default:
		stateVal = Red.Render("● " + m.data.State)
	}

	row("Cluster", stateVal)
	row("Runtime", m.data.Runtime)
	if m.data.Version != "" {
		row("Version", m.data.Version)
	}
	if m.data.Uptime != "" {
		row("Uptime", m.data.Uptime)
	}

	// endpoints
	if m.data.State == "running" {
		var endpoints []string
		if m.data.APIServer != "" {
			endpoints = append(endpoints, fmt.Sprintf("  %s %s", label.Render("API"), m.data.APIServer))
		}
		if m.data.ConsoleURL != "" {
			endpoints = append(endpoints, fmt.Sprintf("  %s %s", label.Render("Console"), m.data.ConsoleURL))
		}
		if m.data.IngressHTTP != "" || m.data.IngressHTTPS != "" {
			parts := []string{}
			if m.data.IngressHTTP != "" {
				parts = append(parts, m.data.IngressHTTP)
			}
			if m.data.IngressHTTPS != "" {
				parts = append(parts, m.data.IngressHTTPS)
			}
			endpoints = append(endpoints, fmt.Sprintf("  %s %s", label.Render("Ingress"), strings.Join(parts, Dim.Render(" | "))))
		}
		if len(endpoints) > 0 {
			b.WriteString("\n")
			box := Box.Render(strings.Join(endpoints, "\n"))
			b.WriteString(indentStr(box, 2))
			b.WriteString("\n")
		}

		// addons
		if len(m.data.Addons) > 0 {
			addonLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(16)
			var rows []string
			for _, a := range m.data.Addons {
				dot := StatusDot(a.Ready)
				state := Green.Render("ready")
				if !a.Ready {
					state = Yellow.Render("not ready")
				}
				rows = append(rows, fmt.Sprintf("  %s %s %s", addonLabel.Render(a.Name), dot, state))
			}
			b.WriteString("\n")
			box := Box.Render(strings.Join(rows, "\n"))
			b.WriteString(indentStr(box, 2))
			b.WriteString("\n")
		}

		// pods viewport
		if m.ready && len(m.data.Pods) > 0 {
			b.WriteString("\n")
			title := Dim.Render(fmt.Sprintf("  pods (%d)", len(m.data.Pods)))
			b.WriteString(title + "\n")
			b.WriteString(m.viewport.View())
			b.WriteString("\n")
		}
	}

	if m.data.Error != "" {
		b.WriteString("\n")
		row("Error", Red.Render(m.data.Error))
	}

	help := Dim.Render("  q quit  r refresh  ↑↓ scroll")
	b.WriteString("\n" + help + "\n")

	return b.String()
}

func (m StatusModel) renderPodTable() string {
	if len(m.data.Pods) == 0 {
		return Dim.Render("  no pods found")
	}

	nameW := 40
	nsW := 30
	readyW := 6
	header := fmt.Sprintf("  %-*s %-*s %-*s %s",
		nsW, "NAMESPACE", nameW, "NAME", readyW, "READY", "STATUS")

	var b strings.Builder
	b.WriteString(Dim.Render(header) + "\n")

	for _, p := range m.data.Pods {
		name := p.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}
		ns := p.Namespace
		if len(ns) > nsW {
			ns = ns[:nsW-1] + "~"
		}

		statusStyle := Green
		switch p.Status {
		case "Pending":
			statusStyle = Yellow
		case "Failed", "Unknown":
			statusStyle = Red
		}

		b.WriteString(fmt.Sprintf("  %-*s %-*s %-*s %s\n",
			nsW, ns, nameW, name, readyW, p.Ready, statusStyle.Render(p.Status)))
	}

	return b.String()
}

func (m StatusModel) doFetch() tea.Cmd {
	fetch := m.fetch
	return func() tea.Msg {
		return refreshMsg{data: fetch()}
	}
}

func (m StatusModel) scheduleRefresh() tea.Cmd {
	interval := m.interval
	fetch := m.fetch
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return refreshMsg{data: fetch()}
	})
}

func indentStr(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = pad + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
