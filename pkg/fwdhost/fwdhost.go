package fwdhost

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/txn2/txeh"
)

// BackupHostFile
func BackupHostFile(hostfile *txeh.Hosts) (string, error) {

	backupHostsPath := HomeDir() + "/hosts.original"
	if _, err := os.Stat(backupHostsPath); os.IsNotExist(err) {
		from, err := os.Open(hostfile.WriteFilePath)
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
		return fmt.Sprintf("Backing up your original hosts file %s to %s\n", hostfile.WriteFilePath, backupHostsPath), nil
	} else {
		return fmt.Sprintf("Original hosts backup already exists at %s\n", backupHostsPath), nil
	}
}

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}