package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Rosé Pine (main). Backgrounds are never painted, so the terminal's own
// (possibly transparent) background always shows through.
const (
	cBase    = "#191724"
	cSurface = "#1f1d2e"
	cOverlay = "#26233a"
	cMuted   = "#6e6a86"
	cSubtle  = "#908caa"
	cText    = "#e0def4"
	cLove    = "#eb6f92"
	cGold    = "#f6c177"
	cRose    = "#ebbcba"
	cPine    = "#31748f"
	cFoam    = "#9ccfd8"
	cIris    = "#c4a7e7"
	cHlLow   = "#21202e"
	cHlMed   = "#403d52"
	cHlHigh  = "#524f67"
)

func fg(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }

func paint(c, s string) string { return fg(c).Render(s) }

func rgb(c string) (float64, float64, float64) {
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(c, "#"), "%02x%02x%02x", &r, &g, &b)
	return float64(r), float64(g), float64(b)
}

// lerp blends two hex colors; t is clamped to [0,1].
func lerp(a, b string, t float64) string {
	t = clamp(t)
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	mix := func(x, y float64) int { return int(math.Round(x + (y-x)*t)) }
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}

func clamp(t float64) float64 { return math.Max(0, math.Min(1, t)) }

func easeOut(t float64) float64 { t = clamp(t); return 1 - math.Pow(1-t, 3) }

func easeInOut(t float64) float64 {
	t = clamp(t)
	if t < .5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

// gradient samples a looping multi-stop palette at position x (any real number).
func gradient(stops []string, x float64) string {
	n := float64(len(stops))
	x = math.Mod(x, 1)
	if x < 0 {
		x++
	}
	p := x * n
	i := int(p)
	return lerp(stops[i%len(stops)], stops[(i+1)%len(stops)], p-float64(i))
}

var shimmer = []string{cRose, cIris, cFoam, cPine, cIris}

// wave paints s with a gradient that flows along the string over time.
func wave(s string, phase, spread float64, bold bool) string {
	var b strings.Builder
	i := 0
	for _, r := range s {
		st := fg(gradient(shimmer, float64(i)*spread-phase))
		if bold {
			st = st.Bold(true)
		}
		b.WriteString(st.Render(string(r)))
		i++
	}
	return b.String()
}
