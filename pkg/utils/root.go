//go:build !windows

package utils

import (
	"os/exec"
	"strconv"
)

// CheckRoot determines if we have administrative privileges.
// This function delegates to the package-level Checker for testability.
func CheckRoot() (bool, error) {
	return Checker.CheckRoot()
}

// CheckRoot implements RootChecker for defaultRootChecker on Unix systems.
func (d *defaultRootChecker) CheckRoot() (bool, error) {
	cmd := exec.Command("id", "-u")

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	i, err := strconv.Atoi(string(output[:len(output)-1]))
	if err != nil {
		return false, err
	}

	if i == 0 {
		return true, nil
	}

	return false, nil
}
