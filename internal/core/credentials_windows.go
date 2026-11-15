//go:build windows

package core

import "errors"

// Windows will keep keys with the Credential Manager or DPAPI. Until then, saving fails
// visibly: no key is ever written to the state file or another plaintext location.
var errCredentialsUnsupported = errors.New("Secure key storage is not available on Windows yet, so this key cannot be saved.")

func LinearKey(string) (string, error)     { return "", nil }
func SaveLinearKey(string, string) error   { return errCredentialsUnsupported }
func RemoveLinearKey(string) error         { return nil }
func OpenRouterKey(string) (string, error) { return "", errCredentialsUnsupported }
func SaveOpenRouterKey(string, string) error {
	return errCredentialsUnsupported
}
func RemoveOpenRouterKey(string) error { return errCredentialsUnsupported }
