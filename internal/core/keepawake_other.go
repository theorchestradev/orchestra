//go:build !darwin

package core

import (
	"os"
	"os/exec"
	"strconv"
)

// systemKeepAwakeCommand holds a systemd idle and sleep inhibitor until Orchestra exits.
func systemKeepAwakeCommand() *exec.Cmd {
	return exec.Command("systemd-inhibit", "--what=idle:sleep", "--who=Orchestra", "--why=Sessions are running",
		"tail", "--pid="+strconv.Itoa(os.Getpid()), "-f", "/dev/null")
}
