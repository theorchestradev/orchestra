package appearance

import (
	"slices"
	"strings"
)

// sortedFamilies drops hidden system families (named with a leading dot) and duplicates, and
// sorts the rest by name.
func sortedFamilies(families []string) []string {
	result := make([]string, 0, len(families))
	for _, family := range families {
		family = strings.TrimSpace(family)
		if family != "" && !strings.HasPrefix(family, ".") && !slices.Contains(result, family) {
			result = append(result, family)
		}
	}
	slices.SortFunc(result, func(a, b string) int {
		if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	return result
}

// parseFontconfigFamilies reads `fc-list family` output, where a line lists a family's localized
// names separated by commas and escapes some characters with backslashes.
func parseFontconfigFamilies(output string) []string {
	var families []string
	for _, line := range strings.Split(output, "\n") {
		name, _, _ := strings.Cut(line, ",")
		families = append(families, strings.ReplaceAll(name, `\`, ""))
	}
	return sortedFamilies(families)
}
