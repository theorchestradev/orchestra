//go:build windows

package core

import (
	"os"
	"os/exec"
	"strconv"
)

// systemKeepAwakeCommand holds a system-required execution state from a PowerShell
// process until Orchestra exits. ES_CONTINUOUS|ES_SYSTEM_REQUIRED applies to the thread
// that set it, and PowerShell runs the loop on one thread.
func systemKeepAwakeCommand() *exec.Cmd {
	script := `Add-Type -Namespace Orchestra -Name Power -MemberDefinition '[DllImport("kernel32.dll")] public static extern uint SetThreadExecutionState(uint esFlags);'; ` +
		`while (Get-Process -Id ` + strconv.Itoa(os.Getpid()) + ` -ErrorAction SilentlyContinue) { [void][Orchestra.Power]::SetThreadExecutionState(0x80000001); Start-Sleep -Seconds 30 }`
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
}
