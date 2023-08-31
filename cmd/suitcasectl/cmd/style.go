package cmd

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
) //nolint:revive

/*
	keyword = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true).
		Render
	important = lipgloss.NewStyle().
			Foreground(lipgloss.Color("201")).
			Bold(true).
			Render
*/

var paragraph = lipgloss.NewStyle().
	Width(78).
	Padding(1, 0, 0, 2).
	Render

func makeGradientRamp(length int) []lipgloss.Color {
	const startColor = "#F967DC"
	const endColor = "#6B50FF"
	var (
		c        = make([]lipgloss.Color, length)
		start, _ = colorful.Hex(startColor)
		end, _   = colorful.Hex(endColor)
	)
	for i := 0; i < length; i++ {
		step := start.BlendLuv(end, float64(i)/float64(length))
		c[i] = lipgloss.Color(step.Hex())
	}
	return c
}

func makeGradientText(baseStyle lipgloss.Style, str string) string {
	const minSize = 3
	if len(str) < minSize {
		return str
	}
	b := strings.Builder{}
	runes := []rune(str)
	for i, c := range makeGradientRamp(len(str)) {
		b.WriteString(baseStyle.Copy().Foreground(c).Render(string(runes[i])))
	}
	return b.String()
}
