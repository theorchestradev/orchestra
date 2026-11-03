//go:build darwin

package appearance

/*
int orchestraReduceTransparency(void);
void orchestraSetTranslucent(int on, double materialOpacity);
*/
import "C"

// SetWindowTranslucent shows the desktop through the main window with native vibrancy. macOS
// Reduce Transparency keeps the window opaque. It reports whether translucency is in effect.
func SetWindowTranslucent(on bool, opacity int) bool {
	on = on && C.orchestraReduceTransparency() == 0
	value := C.int(0)
	materialOpacity := max(0, min(1, float64(opacity-30)/48))
	if on {
		value = 1
	}
	C.orchestraSetTranslucent(value, C.double(materialOpacity))
	return on
}
