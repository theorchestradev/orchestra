//go:build darwin && cgo

package core

func LinearKey(service string) (string, error) { return platformCredential(service, 0, "") }
func SaveLinearKey(service, key string) error {
	_, err := platformCredential(service, 1, key)
	return err
}
func RemoveLinearKey(service string) error { _, err := platformCredential(service, 2, ""); return err }
