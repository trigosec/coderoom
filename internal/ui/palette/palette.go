// Package palette contains shared UI color tokens and small color-related
// helpers. It intentionally has no dependencies on higher-level UI packages to
// avoid import cycles.
package palette

// ColorDeparted is the colour used to render historical output from agents that
// have left or crashed, replacing their assigned colour so their records dim
// rather than lose colour entirely.
const ColorDeparted = "#6b7280"

// File-change diff colours used when rendering patch output in the transcript.
const (
	ColorFileChangeDiffHeader = "#4FC3F7"
	ColorFileChangeHunk       = "#64B5F6"
	ColorFileChangeAdd        = "#66BB6A"
	ColorFileChangeDel        = "#EF5350"
	ColorFileChangeMeta       = "#BA68C8"
)

// agentColors is the ordered set of colours assigned to agents, spread across
// the hue wheel for maximum contrast on dark terminal backgrounds.
var agentColors = []string{
	"#4ade80", // green
	"#60a5fa", // blue
	"#fbbf24", // amber
	"#f472b6", // pink
	"#c084fc", // purple
	"#fb923c", // orange
	"#22d3ee", // cyan
	"#a3e635", // lime
}

// ColorPalette assigns colours sequentially from a fixed set. Once exhausted,
// Next returns an empty string (default terminal colour). Colours are never
// reused within a session.
type ColorPalette struct {
	next int
}

// Next returns the next colour code and the updated palette. If the palette is
// exhausted the colour is empty (default terminal colour).
func (p ColorPalette) Next() (string, ColorPalette) {
	if p.next >= len(agentColors) {
		return "", p
	}
	color := agentColors[p.next]
	p.next++
	return color, p
}
