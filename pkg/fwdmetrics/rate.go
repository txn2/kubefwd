package fwdmetrics

import (
	"sync"
	"time"
)

// DefaultMaxSamples is the default number of samples to keep in history
const DefaultMaxSamples = 60 // 60 seconds of history

// RateCalculator maintains a rolling window of samples for rate calculation
type RateCalculator struct {
	samples      []RateSample
	maxSamples   int
	currentIndex int
	mu           sync.RWMutex
}

// NewRateCalculator creates a new rate calculator with the specified history size
func NewRateCalculator(maxSamples int) *RateCalculator {
	if maxSamples <= 0 {
		maxSamples = DefaultMaxSamples
	}
	return &RateCalculator{
		samples:    make([]RateSample, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

// AddSample records a new sample for rate calculation
func (rc *RateCalculator) AddSample(bytesIn, bytesOut uint64, timestamp time.Time) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	sample := RateSample{
		Timestamp: timestamp,
		BytesIn:   bytesIn,
		BytesOut:  bytesOut,
	}

	if len(rc.samples) < rc.maxSamples {
		rc.samples = append(rc.samples, sample)
	} else {
		rc.samples[rc.currentIndex] = sample
	}
	rc.currentIndex = (rc.currentIndex + 1) % rc.maxSamples
}

// GetInstantRate returns bytes/sec based on last two samples
//
//goland:noinspection DuplicatedCode
func (rc *RateCalculator) GetInstantRate() (rateIn, rateOut float64) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if len(rc.samples) < 2 {
		return 0, 0
	}

	// Get current and previous samples (accounting for ring buffer)
	currIdx := (rc.currentIndex - 1 + len(rc.samples)) % len(rc.samples)
	prevIdx := (rc.currentIndex - 2 + len(rc.samples)) % len(rc.samples)

	curr := rc.samples[currIdx]
	prev := rc.samples[prevIdx]

	duration := curr.Timestamp.Sub(prev.Timestamp).Seconds()
	if duration <= 0 {
		return 0, 0
	}

	// Handle counter wraparound or reset (e.g., pod restart)
	if curr.BytesIn < prev.BytesIn || curr.BytesOut < prev.BytesOut {
		return 0, 0
	}

	rateIn = float64(curr.BytesIn-prev.BytesIn) / duration
	rateOut = float64(curr.BytesOut-prev.BytesOut) / duration
	return
}

// GetAverageRate returns average bytes/sec over last N seconds
//
//goland:noinspection DuplicatedCode
func (rc *RateCalculator) GetAverageRate(windowSeconds int) (rateIn, rateOut float64) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if len(rc.samples) < 2 || windowSeconds <= 0 {
		return 0, 0
	}

	samplesNeeded := windowSeconds + 1
	if samplesNeeded > len(rc.samples) {
		samplesNeeded = len(rc.samples)
	}

	// Get oldest and newest samples in window
	newestIdx := (rc.currentIndex - 1 + len(rc.samples)) % len(rc.samples)
	oldestIdx := (rc.currentIndex - samplesNeeded + len(rc.samples)) % len(rc.samples)

	newest := rc.samples[newestIdx]
	oldest := rc.samples[oldestIdx]

	duration := newest.Timestamp.Sub(oldest.Timestamp).Seconds()
	if duration <= 0 {
		return 0, 0
	}

	// Handle counter wraparound or reset (e.g., pod restart)
	if newest.BytesIn < oldest.BytesIn || newest.BytesOut < oldest.BytesOut {
		return 0, 0
	}

	rateIn = float64(newest.BytesIn-oldest.BytesIn) / duration
	rateOut = float64(newest.BytesOut-oldest.BytesOut) / duration
	return
}

// GetHistory returns recent samples for graphing (oldest first)
func (rc *RateCalculator) GetHistory(count int) []RateSample {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if count <= 0 || len(rc.samples) == 0 {
		return nil
	}

	if count > len(rc.samples) {
		count = len(rc.samples)
	}

	result := make([]RateSample, count)

	for i := 0; i < count; i++ {
		idx := (rc.currentIndex - count + i + len(rc.samples)) % len(rc.samples)
		result[i] = rc.samples[idx]
	}

	return result
}

// SampleCount returns the number of samples currently stored
func (rc *RateCalculator) SampleCount() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.samples)
}
