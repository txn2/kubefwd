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
	maxVal := findMax(values)
	if maxVal == 0 {
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
		level := int(math.Round((values[idx] / maxVal) * 7))
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
	maxVal := values[0]
	for _, v := range values[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}
