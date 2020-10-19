package fwdhost

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/txn2/txeh"
)

// BackupHostFile will write a backup of the pre-modified host file
// the users home directory, if it does already exist.
func BackupHostFile(hostFile *txeh.Hosts) (string, error) {
	homeDirLocation, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	backupHostsPath := homeDirLocation + "/hosts.original"
	if _, err := os.Stat(backupHostsPath); os.IsNotExist(err) {
		from, err := os.Open(hostFile.WriteFilePath)
		if err != nil {
			return "", err
		}
		defer func() { _ = from.Close() }()

		to, err := os.OpenFile(backupHostsPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer func() { _ = to.Close() }()

		_, err = io.Copy(to, from)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Backing up your original hosts file %s to %s\n", hostFile.WriteFilePath, backupHostsPath), nil
	}

	return fmt.Sprintf("Original hosts backup already exists at %s\n", backupHostsPath), nil
}
