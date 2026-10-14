//go:build darwin

package apps

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
char *orchestraApplicationPath(const char *bundleID);
void *orchestraApplicationIcon(const char *path, int size, int *length);
*/
import "C"
import (
	"encoding/base64"
	"errors"
	"os/exec"
	"sync"
	"unsafe"
)

const fileManagerName = "Finder"

// Icons are drawn once per app location for the life of the process.
var (
	iconsMu sync.Mutex
	icons   = map[string]string{}
)

// Installed lists the known apps Launch Services can find, with their icons.
func Installed() []App {
	result := []App{}
	for _, app := range knownApps {
		if bundleID, path := resolve(app); bundleID != "" {
			result = append(result, App{ID: app.id, Name: app.name, Kind: app.kind, Icon: icon(path)})
		}
	}
	return result
}

// resolve returns the first of the app's bundle identifiers that is installed, and its location.
func resolve(app known) (bundleID, path string) {
	for _, id := range app.bundleIDs {
		value := C.CString(id)
		location := C.orchestraApplicationPath(value)
		C.free(unsafe.Pointer(value))
		if location != nil {
			defer C.free(unsafe.Pointer(location))
			return id, C.GoString(location)
		}
	}
	return "", ""
}

func icon(path string) string {
	iconsMu.Lock()
	defer iconsMu.Unlock()
	if data, ok := icons[path]; ok {
		return data
	}
	value := C.CString(path)
	defer C.free(unsafe.Pointer(value))
	var length C.int
	// 32 pixels keeps 16-point icons sharp on Retina displays.
	bytes := C.orchestraApplicationIcon(value, 32, &length)
	data := ""
	if bytes != nil {
		data = "data:image/png;base64," + base64.StdEncoding.EncodeToString(C.GoBytes(bytes, length))
		C.free(bytes)
	}
	icons[path] = data
	return data
}

// Open asks Launch Services to open folder with the app.
func Open(id, folder string) error {
	app, ok := find(id)
	if !ok {
		return errors.New("Unknown app.")
	}
	bundleID, _ := resolve(app)
	if bundleID == "" {
		return errors.New(app.name + " is not installed.")
	}
	if out, err := exec.Command("open", "-b", bundleID, folder).CombinedOutput(); err != nil {
		if len(out) > 0 {
			return errors.New(string(out))
		}
		return err
	}
	return nil
}
