package utils

import (
	"errors"
	"testing"
)

func TestCheckRoot_WithMock(t *testing.T) {
	// Save original checker
	originalChecker := Checker
	defer func() { Checker = originalChecker }()

	tests := []struct {
		name     string
		isRoot   bool
		err      error
		wantRoot bool
		wantErr  bool
	}{
		{
			name:     "root user",
			isRoot:   true,
			err:      nil,
			wantRoot: true,
			wantErr:  false,
		},
		{
			name:     "non-root user",
			isRoot:   false,
			err:      nil,
			wantRoot: false,
			wantErr:  false,
		},
		{
			name:     "error checking root",
			isRoot:   false,
			err:      errors.New("permission denied"),
			wantRoot: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRootChecker{IsRoot: tt.isRoot, Err: tt.err}
			SetChecker(mock)

			gotRoot, gotErr := CheckRoot()

			if gotRoot != tt.wantRoot {
				t.Errorf("CheckRoot() = %v, want %v", gotRoot, tt.wantRoot)
			}

			if (gotErr != nil) != tt.wantErr {
				t.Errorf("CheckRoot() error = %v, wantErr %v", gotErr, tt.wantErr)
			}

			if mock.CallCount != 1 {
				t.Errorf("Expected 1 call, got %d", mock.CallCount)
			}
		})
	}
}

func TestSetChecker_ResetChecker(t *testing.T) {
	// Save original for cleanup
	original := Checker
	defer func() { Checker = original }()

	// Create and set mock
	mock := NewMockRootChecker(true)
	SetChecker(mock)

	if Checker != mock {
		t.Error("SetChecker did not set the mock")
	}

	// Verify mock works
	isRoot, err := CheckRoot()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isRoot {
		t.Error("Expected isRoot to be true")
	}

	// Reset
	ResetChecker()

	// After reset, should be the default type (not the mock)
	if Checker == mock {
		t.Error("ResetChecker did not restore default")
	}

	// Verify it's the correct type
	_, ok := Checker.(*defaultRootChecker)
	if !ok {
		t.Error("ResetChecker did not restore defaultRootChecker")
	}
}

func TestMockRootChecker_GetCallCount(t *testing.T) {
	mock := NewMockRootChecker(true)

	if mock.GetCallCount() != 0 {
		t.Errorf("Expected initial call count 0, got %d", mock.GetCallCount())
	}

	_, _ = mock.CheckRoot()
	_, _ = mock.CheckRoot()
	_, _ = mock.CheckRoot()

	if mock.GetCallCount() != 3 {
		t.Errorf("Expected call count 3, got %d", mock.GetCallCount())
	}
}

func TestMockRootChecker_Reset(t *testing.T) {
	mock := &MockRootChecker{
		IsRoot:    true,
		Err:       errors.New("some error"),
		CallCount: 5,
	}

	mock.Reset()

	if mock.IsRoot {
		t.Error("Reset did not clear IsRoot")
	}
	if mock.Err != nil {
		t.Error("Reset did not clear Err")
	}
	if mock.CallCount != 0 {
		t.Error("Reset did not clear CallCount")
	}
}

func TestMockRootChecker_ThreadSafety(t *testing.T) {
	mock := NewMockRootChecker(true)

	// Run concurrent calls
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			_, _ = mock.CheckRoot()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	if mock.GetCallCount() != 100 {
		t.Errorf("Expected 100 calls, got %d", mock.GetCallCount())
	}
}

func TestNewMockRootChecker(t *testing.T) {
	tests := []struct {
		isRoot   bool
		wantRoot bool
	}{
		{true, true},
		{false, false},
	}

	for _, tt := range tests {
		mock := NewMockRootChecker(tt.isRoot)
		got, err := mock.CheckRoot()
		if err != nil {
			t.Errorf("NewMockRootChecker(%v) returned error: %v", tt.isRoot, err)
		}
		if got != tt.wantRoot {
			t.Errorf("NewMockRootChecker(%v).CheckRoot() = %v, want %v", tt.isRoot, got, tt.wantRoot)
		}
	}
}

// TestDefaultRootChecker_CheckRoot tests the actual system check
func TestDefaultRootChecker_CheckRoot(t *testing.T) {
	// Create a new default checker instance directly
	checker := &defaultRootChecker{}

	// Call the actual CheckRoot method
	isRoot, err := checker.CheckRoot()

	// The test should not error (unless id command is not available)
	if err != nil {
		t.Logf("CheckRoot returned error (may be expected on some systems): %v", err)
	}

	// On most test runs, we're not root
	if isRoot {
		t.Log("Running as root user")
	} else {
		t.Log("Running as non-root user")
	}

	// The important thing is that it returns without panicking
	// and returns a valid boolean
}
