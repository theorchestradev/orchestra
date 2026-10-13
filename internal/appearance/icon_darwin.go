//go:build darwin

package appearance

/*
int orchestraSetAppIcon(const void *bytes, int length);
*/
import "C"
import "unsafe"

// SetAppIcon copies the PNG before scheduling the Dock update on the main thread.
func SetAppIcon(data []byte) bool {
	return len(data) > 0 && C.orchestraSetAppIcon(unsafe.Pointer(&data[0]), C.int(len(data))) != 0
}
