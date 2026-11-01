package core

import (
	"os"
	"os/exec"
	"strconv"
)

// systemKeepAwakeCommand holds an idle sleep assertion until Orchestra exits.
func systemKeepAwakeCommand() *exec.Cmd {
	return exec.Command("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
}
