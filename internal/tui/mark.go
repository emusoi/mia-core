package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/emusoi/mia-core/internal/mark"
)

func (m model) loadingMark() []string {
	seconds := float64(m.spin) * 0.12
	pad := strings.Repeat(" ", max((m.inner()-19)/2, 2))
	lines := []string{""}
	for _, row := range mark.Grid(seconds) {
		lines = append(lines, pad+row)
	}
	return append(lines, "", pad+lipgloss.PlaceHorizontal(19, lipgloss.Center, dimmed.Render("loading")))
}
