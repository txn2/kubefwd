package fwdhost

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/txn2/kubefwd/pkg/fwdip"
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

	to, err := os.OpenFile(backupHostsPath, os.O_RDWR|os.O_CREATE, 0o644)
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
	hostnames := fwdip.GetRegisteredHostnames()
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

func isKubefwdIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	ip = ip.To4()
	if ip == nil || ip[0] != 127 {
		return false
	}
	return ip[1] > 1 || (ip[1] == 1 && ip[2] >= 27)
}

func CountStaleEntries(hostFile *txeh.Hosts) int {
	lines := hostFile.GetHostFileLines()
	count := 0

	for _, line := range lines {
		if line.LineType == txeh.ADDRESS && isKubefwdIP(line.Address) {
			count += len(line.Hostnames)
		}
	}

	return count
}

func PurgeStaleIps(hostFile *txeh.Hosts) (int, error) {
	lines := hostFile.GetHostFileLines()
	var hostsToRemove []string

	for _, line := range lines {
		if line.LineType == txeh.ADDRESS && isKubefwdIP(line.Address) {
			hostsToRemove = append(hostsToRemove, line.Hostnames...)
		}
	}

	if len(hostsToRemove) == 0 {
		return 0, nil
	}

	hostFile.RemoveHosts(hostsToRemove)
	return len(hostsToRemove), hostFile.Save()
}
