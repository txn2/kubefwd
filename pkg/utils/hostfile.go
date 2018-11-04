package utils

import (
	"fmt"
	"io"
	"log"
	"os"

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
