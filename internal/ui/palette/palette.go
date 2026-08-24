// Package palette contains shared UI color tokens and small color-related
// helpers. It intentionally has no dependencies on higher-level UI packages to
// avoid import cycles.
package palette

import (
	"fmt"
	"math"
)

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

const (
	participantLightness = 0.78
	participantChroma    = 0.16
	initialHue           = 142.0
	goldenAngle          = 137.50776405003785
)

// ColorPalette generates perceptually distributed participant colours.
// Colours are never reused within a session.
type ColorPalette struct {
	next int
	used map[string]struct{}
}

// Next returns the next colour code and an independently updated palette.
func (p ColorPalette) Next() (string, ColorPalette) {
	used := cloneColors(p.used)
	for attempt := 0; ; attempt++ {
		color := participantColor(p.next, attempt)
		if _, exists := used[color]; exists {
			continue
		}
		used[color] = struct{}{}
		p.next++
		p.used = used
		return color, p
	}
}

func cloneColors(colors map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(colors)+1)
	for color := range colors {
		clone[color] = struct{}{}
	}
	return clone
}

func participantColor(index, attempt int) string {
	hue := math.Mod(initialHue+float64(index)*goldenAngle+float64(attempt), 360)
	r, g, b := oklchToSRGB(participantLightness, participantChroma, hue)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

func oklchToSRGB(lightness, chroma, hue float64) (uint8, uint8, uint8) {
	radians := hue * math.Pi / 180
	a := chroma * math.Cos(radians)
	b := chroma * math.Sin(radians)

	l := cube(lightness + 0.3963377774*a + 0.2158037573*b)
	m := cube(lightness - 0.1055613458*a - 0.0638541728*b)
	s := cube(lightness - 0.0894841775*a - 1.291485548*b)

	return linearToSRGB(4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		linearToSRGB(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		linearToSRGB(-0.0041960863*l - 0.7034186147*m + 1.707614701*s)
}

func cube(value float64) float64 { return value * value * value }

func linearToSRGB(value float64) uint8 {
	value = min(max(value, 0), 1)
	if value <= 0.0031308 {
		value *= 12.92
	} else {
		value = 1.055*math.Pow(value, 1/2.4) - 0.055
	}
	return uint8(math.Round(value * 255))
}
