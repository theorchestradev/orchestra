//go:build !darwin || !cgo

package core

import "errors"

func OpenRouterKey(string) (string, error) {
	return "", errors.New("OpenRouter key storage requires macOS Keychain.")
}
func SaveOpenRouterKey(string, string) error {
	return errors.New("OpenRouter key storage requires macOS Keychain.")
}
func RemoveOpenRouterKey(string) error {
	return errors.New("OpenRouter key storage requires macOS Keychain.")
}
