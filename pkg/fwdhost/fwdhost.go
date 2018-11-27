package fwdhost

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/cbednarski/hostess"
)

// GetHostFile returns a pointer to a hostess Hostfile object.
// TODO: refactor to use an interface so we can eventually move to a more stable solution
// see: github.com/cbednarski/hostess
// see: https://github.com/txn2/kubefwd/pull/19
func GetHostFile() (*hostess.Hostfile, []error) {
	// prep for refactor
	return hostess.LoadHostfile()
}

// BackupHostFile
func BackupHostFile(hostfile *hostess.Hostfile) (string, error) {

	backupHostsPath := hostfile.Path + ".original"
	if _, err := os.Stat(backupHostsPath); os.IsNotExist(err) {
		from, err := os.Open(hostfile.Path)
		if err != nil {
			return "", err
		}
		defer from.Close()

		to, err := os.OpenFile(backupHostsPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer to.Close()

		_, err = io.Copy(to, from)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Backing up your original hosts file %s to %s\n", hostfile.Path, backupHostsPath), nil
	} else {
		return fmt.Sprintf("Original hosts backup already exists at %s\n", backupHostsPath), nil
	}
}
