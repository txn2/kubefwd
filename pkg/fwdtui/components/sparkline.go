package components

import (
	"math"
	"strings"
)

// SparklineChars are Unicode block characters for sparkline rendering
// Ordered from lowest (▁) to highest (█)
var SparklineChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderSparkline converts rate values to a Unicode sparkline string.
// Values are normalized to the maximum value in the slice.
// Width determines how many characters to output (will sample/interpolate if needed).
func RenderSparkline(values []float64, width int) string {
	if width <= 0 {
		return ""
	}

	if len(values) == 0 {
		return strings.Repeat(string(SparklineChars[0]), width)
	}

	// Find max for normalization
	max := findMax(values)
	if max == 0 {
		return strings.Repeat(string(SparklineChars[0]), width)
	}

	// Build output
	result := make([]rune, width)
	for i := 0; i < width; i++ {
		// Map output position to input index
		idx := i * len(values) / width
		if idx >= len(values) {
			idx = len(values) - 1
		}

		// Normalize value to 0-7 range
		level := int(math.Round((values[idx] / max) * 7))
		if level < 0 {
			level = 0
		}
		if level > 7 {
			level = 7
		}
		result[i] = SparklineChars[level]
	}

	return string(result)
}

// RenderSparklineWithMax renders a sparkline using a specific max value.
// This allows consistent scaling across multiple sparklines.
func RenderSparklineWithMax(values []float64, width int, max float64) string {
	if width <= 0 {
		return ""
	}

	if len(values) == 0 || max == 0 {
		return strings.Repeat(string(SparklineChars[0]), width)
	}

	result := make([]rune, width)
	for i := 0; i < width; i++ {
		idx := i * len(values) / width
		if idx >= len(values) {
			idx = len(values) - 1
		}

		// Normalize value to 0-7 range, capping at max
		normalized := values[idx] / max
		if normalized > 1 {
			normalized = 1
		}
		level := int(math.Round(normalized * 7))
		if level < 0 {
			level = 0
		}
		if level > 7 {
			level = 7
		}
		result[i] = SparklineChars[level]
	}

	return string(result)
}

// findMax returns the maximum value in a slice
func findMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// FindMax is exported version of findMax for use in consistent scaling
func FindMax(values []float64) float64 {
	return findMax(values)
}

// CombinedMax returns the maximum value across two slices
// Useful for scaling in/out sparklines together
func CombinedMax(a, b []float64) float64 {
	maxA := findMax(a)
	maxB := findMax(b)
	if maxA > maxB {
		return maxA
	}
	return maxB
}
