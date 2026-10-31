package core

import (
	"errors"
	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"path/filepath"
	"strings"
	"unicode"
)

// validWorkspaceEmoji accepts no emoji or a single grapheme cluster, so flags, skin tones and
// ZWJ sequences count as one.
func validWorkspaceEmoji(emoji string) bool {
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
	if state.Version != stateVersion {
		return errors.New("Unsupported saved state version.")
	}
	profiles := map[string]bool{}
	for _, p := range state.Profiles {
		if p == nil || validProfile(p) != nil {
			return errors.New("Invalid saved profile.")
		}
		if _, err := uuid.Parse(p.ID); err != nil || profiles[p.ID] {
			return errors.New("Invalid or duplicate saved profile ID.")
		}
		profiles[p.ID] = true
	}
	workspaces := map[string]bool{}
	sessions := map[string]bool{}
	repositories := map[string]bool{}
	for _, p := range state.Workspaces {
		if p == nil {
			return errors.New("Invalid saved workspace.")
		}
		if _, err := uuid.Parse(p.ID); err != nil || workspaces[p.ID] {
			return errors.New("Invalid or duplicate saved workspace ID.")
		}
		workspaces[p.ID] = true
		if strings.TrimSpace(p.Name) == "" || p.Directory != "" || len(p.Repositories) == 0 {
			return errors.New("Invalid workspace name or repositories.")
		}
		repos, folders := map[string]bool{}, map[string]bool{}
		for _, repo := range p.Repositories {
			if repo == nil || strings.TrimSpace(repo.Name) == "" || !filepath.IsAbs(repo.Directory) || folders[repo.Directory] {
				return errors.New("Invalid saved repository name or folder.")
			}
			if _, err := uuid.Parse(repo.ID); err != nil || repositories[repo.ID] {
				return errors.New("Invalid or duplicate saved repository ID.")
			}
			repositories[repo.ID], repos[repo.ID], folders[repo.Directory] = true, true, true
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
			if !repos[sess.RepositoryID] {
				return errors.New("Saved session repository is unavailable.")
			}
			if sess.LoginAgent != "" && (sess.Agent != terminalAgent || loginCommands[sess.LoginAgent] == "") {
				return errors.New("Invalid saved login session.")
			}
		}
		if p.SelectedSessionID != "" && !local[p.SelectedSessionID] {
			return errors.New("Selected session is unavailable.")
		}
	}
	if state.SelectedWorkspaceID != "" && !workspaces[state.SelectedWorkspaceID] {
		return errors.New("Selected workspace is unavailable.")
	}
	return nil
}
