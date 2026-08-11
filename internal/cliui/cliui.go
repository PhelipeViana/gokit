package cliui

import (
	"fmt"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

type Issue struct {
	Message  string
	Solution string
}

type UserError struct {
	Message  string
	Solution string
	Issues   []Issue
}

func (e UserError) Error() string {
	if len(e.Issues) > 0 {
		var parts []string
		for _, issue := range e.Issues {
			parts = append(parts, i18n.Tf("cli_error_with_fix", issue.Message, issue.Solution))
		}
		return strings.Join(parts, "; ")
	}
	return i18n.Tf("cli_error_with_fix", e.Message, e.Solution)
}

func NewUserError(message, solution string) error {
	return UserError{
		Message:  message,
		Solution: solution,
	}
}

func PrintTitle(title string) {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF79C6")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#BD93F9")).
		Padding(0, 1)
	fmt.Println("\n" + style.Render(title) + "\n")
}

func Info(str string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD")).Render(str)
}

func Muted(str string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render(str)
}

func Failure(str string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(str)
}

func Warning(str string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB86C")).Render(str)
}

func Success(str string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(str)
}
