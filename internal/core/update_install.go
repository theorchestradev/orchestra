package core

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// UpdateDownloader saves the named assets of a release into dir.
type UpdateDownloader func(ctx context.Context, version string, assets []string, dir string) error

// GitHubDownload downloads release assets through the user's GitHub CLI login and
// falls back to the public download URLs without gh, like GitHubReleases.
func GitHubDownload(ctx context.Context, version string, assets []string, dir string) error {
	_, err := withFallback(func() (struct{}, error) {
		args := []string{"release", "download", "v" + version, "--repo", UpdateRepository, "--dir", dir, "--clobber"}
		for _, asset := range assets {
			args = append(args, "--pattern", asset)
		}
		home, _ := os.UserHomeDir()
		_, err := runCommandContext(ctx, home, "gh", nil, args...)
		return struct{}{}, err
	}, func() (struct{}, error) { return struct{}{}, publicDownload(ctx, nil, version, assets, dir) })
	return err
}

// systemCommand runs a local tool and includes its output in errors.
func systemCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(out)); detail != "" {
			return out, fmt.Errorf("%s: %s", filepath.Base(name), detail)
		}
		return out, fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return out, nil
}

// stagePrefix names the folders beside the installed app that hold a verified update
// until quitting applies it. Leftovers from an interrupted install are removed at startup.
const stagePrefix = ".orchestra-update-"

// installTarget returns what an update replaces, or why the app cannot update itself.
func installTarget(executable string) (string, string) {
	if executable == "" {
		return "", "Orchestra could not find its own location."
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	target, err := installedPath(executable)
	if err != nil {
		return "", err.Error()
	}
	// 2 is W_OK: replacing the app needs write access to its folder.
	if syscall.Access(filepath.Dir(target), 2) != nil {
		return "", fmt.Sprintf("Orchestra cannot replace itself in %s. Download the new version and install it yourself.", filepath.Dir(target))
	}
	return target, ""
}

func removeStaleStages(target string) {
	stale, _ := filepath.Glob(filepath.Join(filepath.Dir(target), stagePrefix+"*"))
	for _, dir := range stale {
		os.RemoveAll(dir)
	}
}

// Install downloads, verifies and stages the available release. Quitting applies it;
// with restart, Orchestra opens again afterwards. A failed install changes nothing.
func (u *Updater) Install(ctx context.Context, version string, restart bool) UpdateStatus {
	u.mu.Lock()
	switch {
	case !u.status.Installable:
		u.status.InstallError = u.status.InstallNote
	case u.status.Preparing:
	case u.status.Staged == version:
		u.restart = restart
	case u.status.Available == nil || u.status.Available.Version != version:
		u.status.InstallError = "Version " + version + " is not the available update."
	default:
		ctx, cancel := context.WithCancel(ctx)
		u.cancel = cancel
		u.status.Preparing = true
		u.status.InstallError = ""
		status := u.status
		u.mu.Unlock()
		u.emit(status)

		dir, path, err := u.prepare(ctx, version)
		cancelled := ctx.Err() != nil
		cancel()

		u.mu.Lock()
		u.cancel = nil
		u.status.Preparing = false
		switch {
		case cancelled:
			// The user cancelled or the app is quitting; there is nothing to report.
			os.RemoveAll(dir)
		case err != nil:
			u.log.Warn("update install failed", "version", version, "error", err)
			u.status.InstallError = "Could not install the update: " + err.Error()
		default:
			if u.stageDir != "" {
				os.RemoveAll(u.stageDir)
			}
			u.stageDir, u.stagedPath, u.restart = dir, path, restart
			u.status.Staged = version
			u.log.Info("update staged", "version", version)
		}
	}
	status := u.status
	u.mu.Unlock()
	u.emit(status)
	return status
}

// CancelInstall stops a download or verification in progress.
func (u *Updater) CancelInstall() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.cancel != nil {
		u.cancel()
	}
}

func (u *Updater) prepare(ctx context.Context, version string) (string, string, error) {
	download, err := os.MkdirTemp("", "orchestra-download-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(download)
	asset := updateAsset(version)
	if err := u.download(ctx, version, []string{asset, "SHA256SUMS"}, download); err != nil {
		return "", "", err
	}
	archive := filepath.Join(download, asset)
	if err := verifyChecksum(archive, filepath.Join(download, "SHA256SUMS")); err != nil {
		return "", "", err
	}
	// The stage sits beside the installed app so applying it is a rename on one volume.
	dir, err := os.MkdirTemp(filepath.Dir(u.target), stagePrefix)
	if err != nil {
		return "", "", err
	}
	path, err := u.unpack(ctx, archive, dir, version)
	if err == nil {
		err = u.verify(ctx, path, u.target, version)
	}
	if err != nil {
		os.RemoveAll(dir)
		return "", "", err
	}
	return dir, path, nil
}

func verifyChecksum(archive, sums string) error {
	file, err := os.Open(sums)
	if err != nil {
		return err
	}
	defer file.Close()
	want := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filepath.Base(archive) {
			want = strings.ToLower(fields[0])
		}
	}
	if want == "" {
		return errors.New("SHA256SUMS does not list the download.")
	}
	data, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer data.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, data); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return errors.New("The download does not match its SHA-256 checksum.")
	}
	return nil
}

// ApplyStaged replaces the installed app with the staged update and returns the version
// it installed. App.shutdown calls it after Store has stopped every session. If the
// swap fails, the previous app is put back. With restart, Orchestra opens again either way.
func (u *Updater) ApplyStaged() (string, error) {
	u.mu.Lock()
	dir, path, restart, version := u.stageDir, u.stagedPath, u.restart, u.status.Staged
	u.stageDir, u.stagedPath, u.restart = "", "", false
	u.mu.Unlock()
	if path == "" {
		return "", nil
	}
	defer os.RemoveAll(dir)
	err := swapInstalled(u.target, path, filepath.Join(dir, "previous"))
	if err == nil {
		u.log.Info("update installed", "path", u.target)
	}
	if restart {
		if startErr := u.relaunch(os.Getpid(), u.target).Start(); startErr != nil && err == nil {
			err = startErr
		}
	}
	if err != nil {
		return "", err
	}
	return version, nil
}

func swapInstalled(target, update, previous string) error {
	if err := os.Rename(target, previous); err != nil {
		return err
	}
	if err := os.Rename(update, target); err != nil {
		if restoreErr := os.Rename(previous, target); restoreErr != nil {
			return fmt.Errorf("%w; restoring the previous version also failed: %v", err, restoreErr)
		}
		return err
	}
	return nil
}
