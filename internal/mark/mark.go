// Package mark draws mia's mark: ten by ten dots, a few lit, the rest
// lighting and fading on a cycle — what mia shows while it works.
package mark

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	lit    = map[int]bool{3: true, 14: true, 27: true, 31: true, 46: true, 52: true, 68: true, 75: true, 81: true, 99: true}
	movers = map[int]float64{3: -8.59, 7: -1.42, 11: -2.14, 14: -8.14, 16: -4.16, 17: -6.7, 20: -2.53, 24: -11.47, 27: -9.51, 29: -5.69, 31: -8.04, 33: -0.86, 37: -0.07, 46: -8.91, 47: -3.94, 49: -2.31, 52: -10.57, 56: -1.65, 61: -5.38, 64: -6.28, 68: -7.91, 71: -6.65, 75: -11.04, 76: -4.99, 81: -10.32, 91: -6.09, 93: -3.48, 99: -9.64}

	strong = lipgloss.NewStyle().Bold(true)
	dimmed = lipgloss.NewStyle().Faint(true)
)

const cycle = 12.0

// Dot is the mark's i-th dot, seconds into its animation.
func Dot(i int, seconds float64) string {
	begin, moves := movers[i]
	if !moves {
		if lit[i] {
			return strong.Render("●")
		}
		return dimmed.Render("·")
	}
	phase := math.Mod(seconds-begin, cycle) / cycle
	switch {
	case phase >= 0.6 && phase <= 0.95:
		return strong.Render("●")
	case phase >= 0.55 || phase > 0.95:
		return "•"
	}
	return dimmed.Render("·")
}

// Grid is the mark's ten rows, seconds into its animation; 19 cells wide.
func Grid(seconds float64) []string {
	rows := make([]string, 10)
	for row := range rows {
		dots := make([]string, 10)
		for col := range dots {
			dots[col] = Dot(row*10+col, seconds)
		}
		rows[row] = strings.Join(dots, " ")
	}
	return rows
}
