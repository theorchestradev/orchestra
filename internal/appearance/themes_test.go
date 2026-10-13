package appearance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validLight = `["#ffffff","#F4F4F2","#FFFFFF","#2D2D2B","#6A6A65","#DBDBD7","#A95C3F","#F3E3DC"]`
const validDark = `["#2D2D2B","#373735","#41413E","#F9F9F7","#BDBDB9","#50504B","#CC7D5E","#543D34"]`

func writeTheme(t *testing.T, dir, folder, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, folder), 0o755); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, folder, ThemeFile), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadCustomThemesMissingDirectory(t *testing.T) {
	result := LoadCustomThemes(filepath.Join(t.TempDir(), "themes"))
	if len(result.Themes) != 0 || len(result.Errors) != 0 {
		t.Fatalf("expected no themes or errors, got %+v", result)
	}
}

func TestLoadCustomThemesValidAndDefaults(t *testing.T) {
	dir := t.TempDir()
	ansi := `["#000000","#F07178","#B8CC52","#FFB454","#59C2FF","#D2A6FF","#95E6CB","#BFBDB6","#323A4C","#F07178","#B8CC52","#FFB454","#59C2FF","#D2A6FF","#95E6CB","#F3F4F5"]`
	writeTheme(t, dir, "solar-flare", `{"light":`+validLight+`,"dark":`+validDark+`}`)
	writeTheme(t, dir, "dracula", `{"name":"  My   Dracula ","light":`+validLight+`,"dark":`+validDark+`,"ansi":`+ansi+`}`)
	if err := os.WriteFile(filepath.Join(dir, "stray.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTheme(t, dir, ".hidden", "not json")

	result := LoadCustomThemes(dir)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Themes) != 2 {
		t.Fatalf("expected 2 themes, got %+v", result.Themes)
	}
	// Sorted by display name; a folder named like a built-in theme stays prefixed.
	dracula, flare := result.Themes[0], result.Themes[1]
	if dracula.ID != "custom:dracula" || dracula.Name != "My Dracula" || len(dracula.ANSI) != 16 {
		t.Fatalf("unexpected theme %+v", dracula)
	}
	if flare.ID != "custom:solar-flare" || flare.Name != "Solar Flare" || flare.ANSI != nil {
		t.Fatalf("unexpected theme %+v", flare)
	}
	if flare.Light[0] != "#FFFFFF" || len(flare.Dark) != 8 {
		t.Fatalf("colors were not normalized: %v", flare.Light)
	}
}

func TestLoadCustomThemesSkipsInvalidThemes(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "good", `{"light":`+validLight+`,"dark":`+validDark+`}`)
	writeTheme(t, dir, "empty", "")
	writeTheme(t, dir, "broken", `{"light":`)
	writeTheme(t, dir, "no-dark", `{"light":`+validLight+`}`)
	writeTheme(t, dir, "short", `{"light":["#FFFFFF"],"dark":`+validDark+`}`)
	writeTheme(t, dir, "named-color", `{"light":`+strings.Replace(validLight, `"#ffffff"`, `"white"`, 1)+`,"dark":`+validDark+`}`)
	writeTheme(t, dir, "bad-ansi", `{"light":`+validLight+`,"dark":`+validDark+`,"ansi":["#000000"]}`)

	result := LoadCustomThemes(dir)
	if len(result.Themes) != 1 || result.Themes[0].ID != "custom:good" {
		t.Fatalf("expected only the valid theme, got %+v", result.Themes)
	}
	want := map[string]string{
		"empty":       "missing theme.json",
		"broken":      "not valid JSON",
		"no-dark":     `"dark" is required`,
		"short":       `"light" needs 8 colors, found 1`,
		"named-color": `"light" color 1 ("white") is not a #RRGGBB hex color`,
		"bad-ansi":    `"ansi" needs 16 colors`,
	}
	if len(result.Errors) != len(want) {
		t.Fatalf("expected %d errors, got %v", len(want), result.Errors)
	}
	for folder, message := range want {
		found := false
		for _, e := range result.Errors {
			found = found || (strings.HasPrefix(e, folder+": ") && strings.Contains(e, message))
		}
		if !found {
			t.Errorf("missing error for %s containing %q in %v", folder, message, result.Errors)
		}
	}
}

// validateBuiltInThemes applies the custom-theme rules to frontend/src/shared/themes.json and
// also requires a name, which custom themes may derive from their folder.
func validateBuiltInThemes(data []byte) []string {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return []string{err.Error()}
	}
	problems := []string{}
	for id, entry := range entries {
		theme, err := decodeTheme(entry)
		if err != nil {
			problems = append(problems, id+": "+err.Error())
		} else if theme.Name == "" {
			problems = append(problems, id+": missing name")
		}
	}
	return problems
}

func TestBuiltInThemesFollowTheCustomThemeRules(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "shared", "themes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := validateBuiltInThemes(data); len(problems) != 0 {
		t.Fatalf("invalid built-in themes: %v", problems)
	}
	ansi := `["#000000","#F07178","#B8CC52","#FFB454","#59C2FF","#D2A6FF","#95E6CB","#BFBDB6","#323A4C","#F07178","#B8CC52","#FFB454","#59C2FF","#D2A6FF","#95E6CB","#F3F4F5"]`
	for entry, want := range map[string]string{
		`{"name":"Good","light":` + validLight + `,"dark":` + validDark + `,"ansi":` + ansi + `}`:                   "",
		`{"light":` + validLight + `,"dark":` + validDark + `}`:                                                     "missing name",
		`{"name":" ","light":` + validLight + `,"dark":` + validDark + `}`:                                          "missing name",
		`{"name":"Short","light":["#FFFFFF"],"dark":` + validDark + `}`:                                             "needs 8 colors",
		`{"name":"Bad","light":` + strings.Replace(validLight, "#ffffff", "#fff", 1) + `,"dark":` + validDark + `}`: "is not a #RRGGBB",
		`{"name":"ANSI","light":` + validLight + `,"dark":` + validDark + `,"ansi":["#000000"]}`:                    "needs 16 colors",
	} {
		problems := validateBuiltInThemes([]byte(`{"theme":` + entry + `}`))
		if want == "" && len(problems) != 0 || want != "" && (len(problems) != 1 || !strings.Contains(problems[0], want)) {
			t.Errorf("%s: expected %q, got %v", entry, want, problems)
		}
	}
}

func TestDuplicateThemeWritesANewUserOnlyFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	light, dark := strings.Split(strings.Trim(validLight, "[]"), ","), strings.Split(strings.Trim(validDark, "[]"), ",")
	for i := range light {
		light[i], dark[i] = strings.Trim(light[i], `"`), strings.Trim(dark[i], `"`)
	}
	first, err := DuplicateTheme(dir, "  Tokyo   Night Copy ", light, dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "custom:tokyo-night-copy" || first.Name != "Tokyo Night Copy" || first.Light[0] != "#FFFFFF" {
		t.Fatalf("unexpected theme %+v", first)
	}
	second, err := DuplicateTheme(dir, "Tokyo Night Copy", light, dark, nil)
	if err != nil || second.ID != "custom:tokyo-night-copy-2" {
		t.Fatalf("expected a suffixed folder, got %+v, %v", second, err)
	}
	original, _ := os.ReadFile(filepath.Join(dir, "tokyo-night-copy", ThemeFile))
	if !strings.Contains(string(original), `"name": "Tokyo Night Copy"`) {
		t.Fatalf("the first theme changed: %s", original)
	}
	for _, path := range []string{dir, filepath.Join(dir, "tokyo-night-copy"), filepath.Join(dir, "tokyo-night-copy", ThemeFile)} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s is not user-only: %v %v", path, info.Mode(), err)
		}
	}
	result := LoadCustomThemes(dir)
	if len(result.Errors) != 0 || len(result.Themes) != 2 {
		t.Fatalf("duplicated themes do not load: %+v", result)
	}
	if _, err := DuplicateTheme(dir, " ", light, dark, nil); err == nil {
		t.Fatal("expected an error for an empty name")
	}
	if _, err := DuplicateTheme(dir, "Short", light[:3], dark, nil); err == nil {
		t.Fatal("expected an error for invalid colors")
	}
	if theme, err := DuplicateTheme(dir, "★", light, dark, light[:8]); err == nil || theme.ID != "" {
		t.Fatalf("expected an error for 8 ANSI colors, got %+v", theme)
	}
	if theme, _ := DuplicateTheme(dir, "★ ★", light, dark, nil); theme.ID != "custom:theme" {
		t.Fatalf("expected a fallback folder, got %+v", theme)
	}
	if len(LoadCustomThemes(dir).Themes) != 3 {
		t.Fatal("an invalid theme left a folder behind")
	}
}

func TestCustomThemeFolderRejectsOtherIDs(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "mine", `{"light":`+validLight+`,"dark":`+validDark+`}`)
	if path, err := CustomThemeFolder(dir, "custom:mine"); err != nil || path != filepath.Join(dir, "mine") {
		t.Fatalf("unexpected %q, %v", path, err)
	}
	for _, id := range []string{"mine", "dracula", "custom:", "custom:../mine", "custom:a/b", "custom:..", "custom:missing"} {
		if _, err := CustomThemeFolder(dir, id); err == nil {
			t.Errorf("expected an error for %q", id)
		}
	}
}
