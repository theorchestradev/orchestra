package appearance

import (
	"slices"
	"testing"
)

func TestFontFamilies(t *testing.T) {
	got := parseFontconfigFamilies("DejaVu Sans Mono,DejaVu Sans Mono Book\nfira code\n\nJetBrainsMono Nerd Font\\-Mono\n.Hidden Mono\nDejaVu Sans Mono\n")
	want := []string{"DejaVu Sans Mono", "fira code", "JetBrainsMono Nerd Font-Mono"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := sortedFamilies(nil); len(got) != 0 {
		t.Fatalf("empty list gave %q", got)
	}
}

func TestMonospaceFontsDoesNotFail(t *testing.T) {
	// The list depends on the machine; listing must still return without panicking.
	for _, family := range MonospaceFonts() {
		if family == "" {
			t.Fatal("listed an empty family")
		}
	}
}
