//go:build darwin

package appearance

/*
void orchestraSetAppearance(int mode);
*/
import "C"

// SetWindowAppearance sets the app's NSAppearance, which Wails leaves unset on macOS.
// "system" clears it, so the title bar, vibrancy and the webview's prefers-color-scheme follow
// the OS and change with it.
func SetWindowAppearance(mode string) {
	value := C.int(0)
	switch mode {
	case "light":
		value = 1
	case "dark":
		value = 2
	}
	C.orchestraSetAppearance(value)
}
