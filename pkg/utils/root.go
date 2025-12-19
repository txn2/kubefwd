//go:build !windows

package utils

import (
	"os/exec"
	"strconv"
)

// CheckRoot determines if we have administrative privileges.
func CheckRoot() (bool, error) {
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
