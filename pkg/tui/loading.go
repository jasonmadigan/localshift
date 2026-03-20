package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type loadDoneMsg[T any] struct{ val T }

// LoadingModel shows a spinner while a function runs in the background.
type LoadingModel[T any] struct {
	spinner spinner.Model
	message string
	result  T
	done    bool
	fetch   func() T
}

func NewLoadingModel[T any](message string, fetch func() T) LoadingModel[T] {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff69b4"))
	return LoadingModel[T]{
		spinner: s,
		message: message,
		fetch:   fetch,
	}
}

func (m LoadingModel[T]) Init() tea.Cmd {
	fetch := m.fetch
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			return loadDoneMsg[T]{val: fetch()}
		},
	)
}

func (m LoadingModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case loadDoneMsg[T]:
		m.result = msg.val
		m.done = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m LoadingModel[T]) View() string {
	if m.done {
		return ""
	}
	return fmt.Sprintf("\n  %s %s\n", m.spinner.View(), Dim.Render(m.message))
}

func (m LoadingModel[T]) Result() T { return m.result }
