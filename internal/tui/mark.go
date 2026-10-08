package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	markLit    = map[int]bool{3: true, 14: true, 27: true, 31: true, 46: true, 52: true, 68: true, 75: true, 81: true, 99: true}
	markMovers = map[int]float64{3: -8.59, 7: -1.42, 11: -2.14, 14: -8.14, 16: -4.16, 17: -6.7, 20: -2.53, 24: -11.47, 27: -9.51, 29: -5.69, 31: -8.04, 33: -0.86, 37: -0.07, 46: -8.91, 47: -3.94, 49: -2.31, 52: -10.57, 56: -1.65, 61: -5.38, 64: -6.28, 68: -7.91, 71: -6.65, 75: -11.04, 76: -4.99, 81: -10.32, 91: -6.09, 93: -3.48, 99: -9.64}
)

const markCycle = 12.0

func markDot(i int, seconds float64) string {
	begin, moves := markMovers[i]
	if !moves {
		if markLit[i] {
			return strong.Render("●")
		}
		return dimmed.Render("·")
	}
	phase := math.Mod(seconds-begin, markCycle) / markCycle
	switch {
	case phase >= 0.6 && phase <= 0.95:
		return strong.Render("●")
	case phase >= 0.55 || phase > 0.95:
		return "•"
	}
	return dimmed.Render("·")
}

func (m model) loadingMark() []string {
	seconds := float64(m.spin) * 0.12
	pad := strings.Repeat(" ", max((m.inner()-19)/2, 2))
	lines := []string{""}
	for row := 0; row < 10; row++ {
		dots := make([]string, 10)
		for col := 0; col < 10; col++ {
			dots[col] = markDot(row*10+col, seconds)
		}
		lines = append(lines, pad+strings.Join(dots, " "))
	}
	return append(lines, "", pad+lipgloss.PlaceHorizontal(19, lipgloss.Center, dimmed.Render("loading")))
}
