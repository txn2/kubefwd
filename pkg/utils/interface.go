package utils

// RootChecker determines if the process has administrative privileges.
// This interface allows mocking root checks for testing.
type RootChecker interface {
	// CheckRoot returns true if the current process has root/admin privileges.
	CheckRoot() (bool, error)
}

// defaultRootChecker is the production implementation
// that uses OS-specific commands to check privileges.
// The CheckRoot method is implemented in platform-specific files.
type defaultRootChecker struct{}

// Checker is the package-level RootChecker used by the application.
// Replace this with a mock for testing.
var Checker RootChecker = &defaultRootChecker{}

// SetChecker replaces the current RootChecker (for testing).
func SetChecker(c RootChecker) {
	Checker = c
}

// ResetChecker restores the default RootChecker.
func ResetChecker() {
	Checker = &defaultRootChecker{}
}
