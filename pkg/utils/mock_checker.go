package utils

import "sync"

// MockRootChecker is a test double for RootChecker.
// It allows configuring return values and tracks calls.
type MockRootChecker struct {
	mu sync.Mutex

	// IsRoot determines what CheckRoot returns
	IsRoot bool

	// Err is the error to return from CheckRoot
	Err error

	// CallCount tracks how many times CheckRoot was called
	CallCount int
}

// NewMockRootChecker creates a mock that returns isRoot with no error
func NewMockRootChecker(isRoot bool) *MockRootChecker {
	return &MockRootChecker{IsRoot: isRoot}
}

// CheckRoot implements RootChecker.
func (m *MockRootChecker) CheckRoot() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallCount++
	return m.IsRoot, m.Err
}

// GetCallCount returns the number of times CheckRoot was called
func (m *MockRootChecker) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CallCount
}

// Reset clears the call count and restores default values
func (m *MockRootChecker) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallCount = 0
	m.IsRoot = false
	m.Err = nil
}
