//go:build darwin

package appearance

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
char *orchestraSystemPalette(int dark);
*/
import "C"
import (
	"encoding/json"
	"unsafe"
)

func SystemPalettes() map[string][]string {
	result := map[string][]string{}
	for _, mode := range []string{"dark", "light"} {
		dark := C.int(0)
		if mode == "dark" {
			dark = 1
		}
		value := C.orchestraSystemPalette(dark)
		if value == nil {
			continue
		}
		var colors []string
		_ = json.Unmarshal([]byte(C.GoString(value)), &colors)
		C.free(unsafe.Pointer(value))
		if len(colors) == 8 {
			result[mode] = colors
		}
	}
	return result
}
