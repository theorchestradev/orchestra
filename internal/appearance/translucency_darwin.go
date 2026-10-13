//go:build darwin

package appearance

/*
int orchestraReduceTransparency(void);
void orchestraSetTranslucent(int on);
*/
import "C"

// SetWindowTranslucent shows the desktop through the main window with native vibrancy. macOS
// Reduce Transparency keeps the window opaque. It reports whether translucency is in effect.
func SetWindowTranslucent(on bool) bool {
	on = on && C.orchestraReduceTransparency() == 0
	value := C.int(0)
	if on {
		value = 1
	}
	C.orchestraSetTranslucent(value)
	return on
}
