//go:build !darwin

package appearance

func SetAppIcon([]byte) bool { return false }
