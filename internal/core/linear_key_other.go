//go:build !darwin || !cgo

package core

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// secret-tool uses the desktop Secret Service; no plaintext fallback is written.
func linearSecret(service, operation, key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{operation}
	if operation == "store" {
		args = append(args, "--label=Orchestra Linear API key")
	}
	args = append(args, "service", service, "account", "linear-api-key")
	cmd := exec.CommandContext(ctx, "secret-tool", args...)
	if operation == "store" {
		cmd.Stdin = strings.NewReader(key)
	}
	var out, stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		// Lookup/clear exit 1 without output when the item does not exist.
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && (operation == "lookup" || operation == "clear") && stderr.Len() == 0 {
			return "", nil
		}
		return "", errors.New("Could not access Linear secure storage. Install secret-tool and unlock the desktop keyring.")
	}
	return strings.TrimSpace(out.String()), nil
}
func LinearKey(service string) (string, error) { return linearSecret(service, "lookup", "") }
func SaveLinearKey(service, key string) error {
	_, err := linearSecret(service, "store", key)
	return err
}
func RemoveLinearKey(service string) error { _, err := linearSecret(service, "clear", ""); return err }
