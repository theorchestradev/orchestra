package core

import (
	"errors"
	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"path/filepath"
	"strings"
	"unicode"
)

// validProjectEmoji accepts no emoji or a single grapheme cluster, so flags, skin tones and
// ZWJ sequences count as one.
func validProjectEmoji(emoji string) bool {
	if emoji == "" {
		return true
	}
	if len(emoji) > 64 || uniseg.GraphemeClusterCount(emoji) != 1 {
		return false
	}
	for _, r := range emoji {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validateState(state State) error {
	if state.Version != 1 {
		return errors.New("Unsupported saved workspace version.")
	}
	projects := map[string]bool{}
	sessions := map[string]bool{}
	for _, p := range state.Projects {
		if p == nil {
			return errors.New("Invalid saved project.")
		}
		if _, err := uuid.Parse(p.ID); err != nil || projects[p.ID] {
			return errors.New("Invalid or duplicate saved project ID.")
		}
		projects[p.ID] = true
		if strings.TrimSpace(p.Name) == "" || !filepath.IsAbs(p.Directory) {
			return errors.New("Invalid project name or folder.")
		}
		local := map[string]bool{}
		for _, sess := range p.Sessions {
			if sess == nil {
				return errors.New("Invalid saved session.")
			}
			if _, err := uuid.Parse(sess.ID); err != nil || sessions[sess.ID] {
				return errors.New("Invalid or duplicate saved session ID.")
			}
			sessions[sess.ID] = true
			local[sess.ID] = true
			if _, ok := sessionKindName(sess.Agent); !ok || strings.TrimSpace(sess.Name) == "" {
				return errors.New("Invalid saved agent or session name.")
			}
			if (sess.WorktreeDirectory == "") != (sess.WorktreeBranch == "") || (sess.WorktreeDirectory != "" && !filepath.IsAbs(sess.WorktreeDirectory)) {
				return errors.New("Invalid saved session worktree.")
			}
			if sess.WorktreeBase != "" && (sess.WorktreeDirectory == "" || strings.TrimLeft(sess.WorktreeBase, "0123456789abcdef") != "") {
				return errors.New("Invalid saved session worktree base.")
			}
			if validateLaunch(sess.Agent, sess.Model, sess.Effort) != nil {
				return errors.New("Invalid saved session model or effort.")
			}
			if sess.GitHubItem != nil && sess.GitHubItem.Number <= 0 {
				return errors.New("Invalid saved session GitHub item.")
			}
		}
		if p.SelectedSessionID != "" && !local[p.SelectedSessionID] {
			return errors.New("Selected session is unavailable.")
		}
	}
	if state.SelectedProjectID != "" && !projects[state.SelectedProjectID] {
		return errors.New("Selected project is unavailable.")
	}
	return nil
}
