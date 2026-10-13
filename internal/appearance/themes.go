package appearance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// CustomThemePrefix keeps custom theme IDs apart from built-in ones, so a custom theme
// never replaces a built-in theme with the same folder name.
const CustomThemePrefix = "custom:"

// ThemeFile is the file read from each folder in the custom themes directory.
const ThemeFile = "theme.json"

// maxThemeFileSize bounds a theme file; real files are a few hundred bytes.
const maxThemeFileSize = 64 << 10

// CustomTheme has the shape of an entry in frontend/src/shared/themes.json, plus its ID
// and display name. Light and dark hold canvas, sidebar, surface, text, muted, rule,
// accent and selection, in that order.
type CustomTheme struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Light []string `json:"light"`
	Dark  []string `json:"dark"`
	ANSI  []string `json:"ansi,omitempty"`
}

// CustomThemes lists the valid themes in a directory and explains why others were skipped.
type CustomThemes struct {
	Directory string        `json:"directory"`
	Themes    []CustomTheme `json:"themes"`
	Errors    []string      `json:"errors"`
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// LoadCustomThemes reads <dir>/<theme-name>/theme.json. A missing directory has no themes.
// Invalid themes are skipped with a message; they never prevent the others from loading.
func LoadCustomThemes(dir string) CustomThemes {
	result := CustomThemes{Directory: dir, Themes: []CustomTheme{}, Errors: []string{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			result.Errors = append(result.Errors, fmt.Sprintf("Could not read %s: %v", dir, err))
		}
		return result
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			continue
		}
		theme, err := loadCustomTheme(filepath.Join(dir, name, ThemeFile), name)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		result.Themes = append(result.Themes, theme)
	}
	sort.SliceStable(result.Themes, func(i, j int) bool {
		return strings.ToLower(result.Themes[i].Name) < strings.ToLower(result.Themes[j].Name)
	})
	return result
}

func loadCustomTheme(path, folder string) (CustomTheme, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return CustomTheme{}, fmt.Errorf("missing %s.", ThemeFile)
	}
	if err != nil {
		return CustomTheme{}, err
	}
	if info.Size() > maxThemeFileSize {
		return CustomTheme{}, fmt.Errorf("%s is larger than 64 KB.", ThemeFile)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CustomTheme{}, err
	}
	theme, err := decodeTheme(data)
	if err != nil {
		return CustomTheme{}, err
	}
	theme.ID = CustomThemePrefix + folder
	if theme.Name == "" {
		theme.Name = displayName(folder)
	}
	return theme, nil
}

// decodeTheme validates a theme.json or a frontend/src/shared/themes.json entry. Built-in and
// custom themes share these rules. The name is trimmed and may be empty; colors are uppercased.
func decodeTheme(data []byte) (CustomTheme, error) {
	var file struct {
		Name  *string   `json:"name"`
		Light *[]string `json:"light"`
		Dark  *[]string `json:"dark"`
		ANSI  *[]string `json:"ansi"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return CustomTheme{}, fmt.Errorf("%s is not valid JSON: %v", ThemeFile, err)
	}
	var theme CustomTheme
	if file.Name != nil {
		theme.Name = strings.Join(strings.Fields(*file.Name), " ")
	}
	for _, mode := range []struct {
		key    string
		colors *[]string
		target *[]string
	}{{"light", file.Light, &theme.Light}, {"dark", file.Dark, &theme.Dark}} {
		if mode.colors == nil {
			return CustomTheme{}, fmt.Errorf(`"%s" is required.`, mode.key)
		}
		colors, err := validColors(mode.key, *mode.colors, 8)
		if err != nil {
			return CustomTheme{}, err
		}
		*mode.target = colors
	}
	if file.ANSI != nil {
		colors, err := validColors("ansi", *file.ANSI, 16)
		if err != nil {
			return CustomTheme{}, err
		}
		theme.ANSI = colors
	}
	return theme, nil
}

// DuplicateTheme writes a new <dir>/<slug>/theme.json and returns the theme as it loads. It never
// replaces a folder: a taken slug gets a -2, -3, … suffix. Folders and files are user-only.
func DuplicateTheme(dir, name string, light, dark, ansi []string) (CustomTheme, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return CustomTheme{}, errors.New("Enter a theme name.")
	}
	file := struct {
		Name  string   `json:"name"`
		Light []string `json:"light"`
		Dark  []string `json:"dark"`
		ANSI  []string `json:"ansi,omitempty"`
	}{Name: name, Light: light, Dark: dark, ANSI: ansi}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return CustomTheme{}, err
	}
	if _, err := decodeTheme(data); err != nil {
		return CustomTheme{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return CustomTheme{}, err
	}
	base := slug(name)
	for i := 1; ; i++ {
		folder := base
		if i > 1 {
			folder = fmt.Sprintf("%s-%d", base, i)
		}
		err := os.Mkdir(filepath.Join(dir, folder), 0o700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return CustomTheme{}, err
		}
		path := filepath.Join(dir, folder, ThemeFile)
		if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
			os.RemoveAll(filepath.Join(dir, folder))
			return CustomTheme{}, err
		}
		return loadCustomTheme(path, folder)
	}
}

// CustomThemeFolder returns the folder of a custom theme ID, or an error for any other ID.
func CustomThemeFolder(dir, id string) (string, error) {
	folder, ok := strings.CutPrefix(id, CustomThemePrefix)
	if !ok || folder == "" || folder != filepath.Base(folder) || strings.HasPrefix(folder, ".") {
		return "", fmt.Errorf("%q is not a custom theme.", id)
	}
	path := filepath.Join(dir, folder)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return "", fmt.Errorf("The theme folder %s no longer exists.", path)
	}
	return path, nil
}

// slug turns "Tokyo Night Copy" into "tokyo-night-copy" for a theme folder.
func slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	if b.Len() == 0 {
		return "theme"
	}
	return b.String()
}

func validColors(key string, colors []string, count int) ([]string, error) {
	if len(colors) != count {
		return nil, fmt.Errorf(`"%s" needs %d colors, found %d.`, key, count, len(colors))
	}
	result := make([]string, count)
	for i, color := range colors {
		if !hexColor.MatchString(color) {
			return nil, fmt.Errorf(`"%s" color %d (%q) is not a #RRGGBB hex color.`, key, i+1, color)
		}
		result[i] = strings.ToUpper(color)
	}
	return result, nil
}

// displayName turns a folder such as "solar-flare" into "Solar Flare".
func displayName(folder string) string {
	words := strings.FieldsFunc(folder, func(r rune) bool { return r == '-' || r == '_' || unicode.IsSpace(r) })
	for i, word := range words {
		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	if len(words) == 0 {
		return folder
	}
	return strings.Join(words, " ")
}
