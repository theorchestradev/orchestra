//go:build darwin

package appearance

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
char *orchestraMonospaceFonts(void);
*/
import "C"
import (
	"encoding/json"
	"unsafe"
)

// MonospaceFonts lists the installed fixed-pitch font families, or nil when AppKit cannot.
func MonospaceFonts() []string {
	value := C.orchestraMonospaceFonts()
	if value == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(value))
	var families []string
	if json.Unmarshal([]byte(C.GoString(value)), &families) != nil {
		return nil
	}
	return sortedFamilies(families)
}
