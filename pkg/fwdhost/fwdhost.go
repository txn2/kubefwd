package fwdhost

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/cbednarski/hostess"
)

// GetHostFile returns a pointer to a hostess Hostfile object.
// TODO: refactor to use an interface so we can eventually move to a more stable solution
// see: github.com/cbednarski/hostess
// see: https://github.com/txn2/kubefwd/pull/19
func GetHostFile() (*hostess.Hostfile, []error) {

	// prep for refactor
	// capture duplicate localhost here
	// TODO need a better solution, this is a hack
	hf, errs := hostess.LoadHostfile()

	for _, err := range errs {
		if err.Error() == "Duplicate hostname entry for localhost -> ::1" {
			_, err = BackupHostFile(hf)
			if err != nil {
				return hf, []error{errors.New("Could not back up hostfile.")}
			}

			// fix the duplicate
			input, err := ioutil.ReadFile(hf.Path)
			if err != nil {
				return hf, []error{err}
			}

			lines := strings.Split(string(input), "\n")
			for i, line := range lines {
				// if the line looks something like this then it's
				// probably the fault of hostess on a previous run and
				// safe to fix.
				if strings.Contains(line, "::1 localhost localhost") {
					lines[i] = "::1 localhost"
				}
			}

			output := strings.Join(lines, "\n")
			err = ioutil.WriteFile(hf.Path, []byte(output), 0644)
			if err != nil {
				return hf, []error{err}
			}

			return hostess.LoadHostfile()

		}
	}

	return hf, errs
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
