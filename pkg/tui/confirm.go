package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type ConfirmModel struct {
	prompt    string
	confirmed bool
	answered  bool
}

func NewConfirmModel(prompt string) ConfirmModel {
	return ConfirmModel{prompt: prompt}
}

func (m ConfirmModel) Init() tea.Cmd { return nil }

func (m ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			m.answered = true
			return m, tea.Quit
		case "n", "N", "q", "ctrl+c", "esc":
			m.confirmed = false
			m.answered = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ConfirmModel) View() string {
	return fmt.Sprintf("\n  %s %s ", m.prompt, Dim.Render("[y/N]"))
}

func (m ConfirmModel) Confirmed() bool { return m.confirmed }

// Confirm shows a y/N prompt. Returns true if the user pressed y.
func Confirm(prompt string) (bool, error) {
	if !IsTTY() {
		return false, fmt.Errorf("cannot confirm in non-interactive mode")
	}
	m := NewConfirmModel(prompt)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	fmt.Println()
	return final.(ConfirmModel).Confirmed(), nil
}
