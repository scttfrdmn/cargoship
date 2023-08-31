package cmd

import (
	"github.com/charmbracelet/lipgloss"
) //nolint:revive

var (
	keyword = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true).
		Render
	important = lipgloss.NewStyle().
			Foreground(lipgloss.Color("201")).
			Bold(true).
			Render

	paragraph = lipgloss.NewStyle().
			Width(78).
			Padding(1, 0, 0, 2).
			Render
)
