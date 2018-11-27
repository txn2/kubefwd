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
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/cbednarski/hostess"
)

// GetHostFile returns a pointer to a hostess Hostfile object.
func GetHostFile() (*hostess.Hostfile, error) {
	hostfile, errs := hostess.LoadHostfile()

	fmt.Printf("Loading hosts file %s\n", hostfile.Path)

	if errs != nil {
		fmt.Println("Can not load /etc/hosts")
		for _, err := range errs {
			return hostfile, err
		}
	}

	// make backup of original hosts file if no previous backup exists
	backupHostsPath := hostfile.Path + ".original"
	if _, err := os.Stat(backupHostsPath); os.IsNotExist(err) {
		from, err := os.Open(hostfile.Path)
		if err != nil {
			return hostfile, err
		}
		defer from.Close()

		to, err := os.OpenFile(backupHostsPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer to.Close()

		_, err = io.Copy(to, from)
		if err != nil {
			return hostfile, err
		}
		fmt.Printf("Backing up your original hosts file %s to %s\n", hostfile.Path, backupHostsPath)
	} else {
		fmt.Printf("Original hosts backup already exists at %s\n", backupHostsPath)
	}

	return hostfile, nil
}

// CheckRoot determines if we have administrative privileges.
func CheckRoot() bool {
	if runtime.GOOS == "windows" {
		// try to open a file which requires admin privileges
		file, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		defer file.Close()
		return err == nil
	}

	cmd := exec.Command("id", "-u")

	output, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
		return false
	}

	i, err := strconv.Atoi(string(output[:len(output)-1]))
	if err != nil {
		log.Fatal(err)
		return false
	}

	if i == 0 {
		return true
	}

	return false
}
