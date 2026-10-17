package fwdapi

import (
	"errors"
	"net"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// stubCommands replaces goos and runCommand for one test, recording each
// command run. The runner returns err for every call.
func stubCommands(t *testing.T, platform string, err error) *[][]string {
	t.Helper()
	origGOOS, origRun := goos, runCommand
	t.Cleanup(func() { goos, runCommand = origGOOS, origRun })

	var calls [][]string
	goos = platform
	runCommand = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if err != nil {
			return []byte("command output"), err
		}
		return nil, nil
	}
	return &calls
}

// exitError returns a real *exec.ExitError with the given status.
func exitError(t *testing.T, code int) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell to produce an exit status")
	}
	err := exec.CommandContext(t.Context(), "sh", "-c", "exit "+strconv.Itoa(code)).Run() // #nosec G204 -- code is an int from the test
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != code {
		t.Fatalf("could not produce exit status %d: %v", code, err)
	}
	return err
}

func TestLoopbackAlias_Commands(t *testing.T) {
	ip := net.ParseIP("127.2.27.254")
	testCases := []struct {
		platform string
		add      []string
		remove   []string
	}{
		{
			platform: "darwin",
			add:      []string{"ifconfig", "lo0", "alias", "127.2.27.254", "up"},
			remove:   []string{"ifconfig", "lo0", "-alias", "127.2.27.254"},
		},
		{
			platform: "linux",
			add:      []string{"ip", "addr", "add", "127.2.27.254/32", "dev", "lo"},
			remove:   []string{"ip", "addr", "del", "127.2.27.254/32", "dev", "lo"},
		},
		{
			platform: "windows",
			add:      []string{"netsh", "interface", "ip", "add", "address", "Loopback Pseudo-Interface 1", "127.2.27.254", "255.255.255.255"},
			remove:   []string{"netsh", "interface", "ip", "delete", "address", "Loopback Pseudo-Interface 1", "127.2.27.254"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.platform, func(t *testing.T) {
			calls := stubCommands(t, tc.platform, nil)

			if err := addLoopbackAlias(ip); err != nil {
				t.Fatalf("addLoopbackAlias: %v", err)
			}
			if err := removeLoopbackAlias(ip); err != nil {
				t.Fatalf("removeLoopbackAlias: %v", err)
			}

			want := [][]string{tc.add, tc.remove}
			if !reflect.DeepEqual(*calls, want) {
				t.Errorf("commands = %v, want %v", *calls, want)
			}
		})
	}
}

func TestLoopbackAlias_CommandFailure(t *testing.T) {
	ip := net.ParseIP("127.2.27.254")
	for _, platform := range []string{"darwin", "linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			stubCommands(t, platform, errors.New("boom"))

			err := addLoopbackAlias(ip)
			if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "command output") {
				t.Errorf("addLoopbackAlias error = %v, want the command error and its output", err)
			}
			err = removeLoopbackAlias(ip)
			if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "command output") {
				t.Errorf("removeLoopbackAlias error = %v, want the command error and its output", err)
			}
		})
	}
}

// On Linux, iproute2 exits 2 when the address already exists (add) or is
// already gone (delete); both leave the loopback as the caller wants it.
func TestLoopbackAlias_LinuxAlreadyInState(t *testing.T) {
	ip := net.ParseIP("127.2.27.254")
	stubCommands(t, "linux", exitError(t, ipExistsExitCode))

	if err := addLoopbackAlias(ip); err != nil {
		t.Errorf("addLoopbackAlias with exit 2 = %v, want nil", err)
	}
	if err := removeLoopbackAlias(ip); err != nil {
		t.Errorf("removeLoopbackAlias with exit 2 = %v, want nil", err)
	}
}

func TestLoopbackAlias_LinuxOtherExitCodeFails(t *testing.T) {
	ip := net.ParseIP("127.2.27.254")
	stubCommands(t, "linux", exitError(t, 1))

	if err := addLoopbackAlias(ip); err == nil {
		t.Error("addLoopbackAlias with exit 1 = nil, want an error")
	}
	if err := removeLoopbackAlias(ip); err == nil {
		t.Error("removeLoopbackAlias with exit 1 = nil, want an error")
	}
}

func TestLoopbackAlias_UnsupportedPlatform(t *testing.T) {
	calls := stubCommands(t, "plan9", nil)
	ip := net.ParseIP("127.2.27.254")

	if err := addLoopbackAlias(ip); err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Errorf("addLoopbackAlias error = %v, want unsupported platform plan9", err)
	}
	if err := removeLoopbackAlias(ip); err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Errorf("removeLoopbackAlias error = %v, want unsupported platform plan9", err)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %v on an unsupported platform, want no commands", *calls)
	}
}

func TestRemoveLoopbackAlias_NilIP(t *testing.T) {
	calls := stubCommands(t, "linux", nil)

	if err := removeLoopbackAlias(nil); err != nil {
		t.Errorf("removeLoopbackAlias(nil) = %v, want nil", err)
	}
	if len(*calls) != 0 {
		t.Errorf("ran %v for a nil IP, want no commands", *calls)
	}
}

func TestAlreadyInState(t *testing.T) {
	if alreadyInState(errors.New("plain")) {
		t.Error("alreadyInState matched an error that is not an exit")
	}
	if alreadyInState(nil) {
		t.Error("alreadyInState matched a nil error")
	}
	if alreadyInState(exitError(t, 1)) {
		t.Error("alreadyInState matched exit status 1")
	}
	if !alreadyInState(exitError(t, ipExistsExitCode)) {
		t.Errorf("alreadyInState did not match exit status %d", ipExistsExitCode)
	}
}
