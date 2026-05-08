package ui

// ColorDeparted is the colour used to render historical output from agents that
// have left or crashed, replacing their assigned colour so their records dim
// rather than lose colour entirely.
const ColorDeparted = "#6b7280"

// paletteColors is the ordered set of colours assigned to agents, spread across
// the hue wheel for maximum contrast on dark terminal backgrounds.
var paletteColors = []string{
	"#4ade80", // green
	"#60a5fa", // blue
	"#fbbf24", // amber
	"#f472b6", // pink
	"#c084fc", // purple
	"#fb923c", // orange
	"#22d3ee", // cyan
	"#a3e635", // lime
}

// colorPalette assigns colours sequentially from a fixed set. Once exhausted,
// Next returns an empty string (default terminal colour). Colours are never
// reused within a session.
type colorPalette struct {
	next int
}

// Next returns the next colour code and the updated palette. If the palette is
// exhausted the colour is empty (default terminal colour).
func (p colorPalette) Next() (string, colorPalette) {
	if p.next >= len(paletteColors) {
		return "", p
	}
	color := paletteColors[p.next]
	p.next++
	return color, p
}
