package fwdhost

import (
	"fmt"
	"io"
	"os"

	"github.com/txn2/kubefwd/pkg/fwdIp"
	"github.com/txn2/txeh"
)

func BackupHostFile(hostFile *txeh.Hosts, forceRefresh bool) (string, error) {
	homeDirLocation, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	backupHostsPath := homeDirLocation + "/hosts.original"
	if info, err := os.Stat(backupHostsPath); err == nil {
		if !forceRefresh {
			return fmt.Sprintf("Original hosts backup already exists at %s (created %s). Use -b to create a fresh backup.\n", backupHostsPath, info.ModTime().Format("2006-01-02 15:04:05")), nil
		}
		if err := os.Remove(backupHostsPath); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	from, err := os.Open(hostFile.WriteFilePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = from.Close() }()

	to, err := os.OpenFile(backupHostsPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}
	defer func() { _ = to.Close() }()

	_, err = io.Copy(to, from)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Backing up hosts file %s to %s\n", hostFile.WriteFilePath, backupHostsPath), nil
}

func RemoveAllocatedHosts() error {
	hostnames := fwdIp.GetRegisteredHostnames()
	if len(hostnames) == 0 {
		return nil
	}

	hosts, err := txeh.NewHostsDefault()
	if err != nil {
		return err
	}

	hosts.RemoveHosts(hostnames)
	return hosts.Save()
}
