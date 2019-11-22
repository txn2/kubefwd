// +build !windows

/*
Copyright 2018 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
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
