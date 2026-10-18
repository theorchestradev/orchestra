//go:build darwin

package appearance

/*
#include <stddef.h>
void orchestraSetAppIcon(const void *bytes, size_t length);
*/
import "C"
import "unsafe"

// SetAppIcon changes the Dock icon without macOS adding a background around the PNG.
func SetAppIcon(png []byte) {
	if len(png) > 0 {
		C.orchestraSetAppIcon(unsafe.Pointer(&png[0]), C.size_t(len(png)))
	}
}
