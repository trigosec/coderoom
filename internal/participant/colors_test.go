package participant

import (
	"fmt"
	"math"
	"regexp"
	"testing"
)

const (
	darkBackground     = "#111827"
	minimumContrast    = 4.5
	readabilitySamples = 1_000
)

func TestColorAllocator_generatesUniqueHTMLColors(t *testing.T) {
	allocator := colorAllocator{}
	seen := make(map[string]bool)
	valid := regexp.MustCompile(`^#[0-9A-F]{6}$`)

	for range 1_000 {
		color := allocator.next()
		if !valid.MatchString(color) {
			t.Fatalf("expected HTML color, got %q", color)
		}
		if seen[color] {
			t.Fatalf("expected unique color, got %q twice", color)
		}
		seen[color] = true
	}
}

func TestColorAllocator_isDeterministic(t *testing.T) {
	first := colorAllocator{}
	second := colorAllocator{}
	for range 100 {
		firstColor := first.next()
		secondColor := second.next()
		if firstColor != secondColor {
			t.Fatalf("expected equal colors, got %q and %q", firstColor, secondColor)
		}
	}
}

func TestColorAllocator_isReadableOnDarkBackground(t *testing.T) {
	allocator := colorAllocator{}
	for range readabilitySamples {
		color := allocator.next()
		if contrastRatio(color, darkBackground) < minimumContrast {
			t.Fatalf("expected %s to have at least %.1f:1 contrast against %s", color, minimumContrast, darkBackground)
		}
	}
}

func contrastRatio(foreground, background string) float64 {
	foregroundLuminance := relativeLuminance(foreground)
	backgroundLuminance := relativeLuminance(background)
	lighter := max(foregroundLuminance, backgroundLuminance)
	darker := min(foregroundLuminance, backgroundLuminance)
	return (lighter + 0.05) / (darker + 0.05)
}

func relativeLuminance(color string) float64 {
	var red, green, blue uint8
	if _, err := fmt.Sscanf(color, "#%02X%02X%02X", &red, &green, &blue); err != nil {
		panic(err)
	}
	return 0.2126*linearChannel(red) + 0.7152*linearChannel(green) + 0.0722*linearChannel(blue)
}

func linearChannel(channel uint8) float64 {
	value := float64(channel) / 255
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}
