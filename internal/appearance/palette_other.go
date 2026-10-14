//go:build !darwin

package appearance

func SystemPalettes() map[string][]string {
	return map[string][]string{"dark": {"#1E1E1E", "#1E1E1E", "#1E1E1E", "#FFFFFF", "#FFFFFF", "#393B3F", "#007AFF", "#34363A"}, "light": {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#000000", "#000000", "#DDE0E4", "#007AFF", "#E6E8EB"}}
}

func SetWindowTranslucent(bool) bool { return false }

func SetWindowAppearance(string) {}
