package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeInstall is an installed app in a temporary folder: a bundle on macOS, a binary
// elsewhere. Its contents identify the version.
type fakeInstall struct {
	t          *testing.T
	dir        string
	target     string
	executable string
	payload    string // what the next download contains
	sums       string // overrides SHA256SUMS when set
	verifyErr  error
	relaunched []string
	download   func(context.Context) error
}

func newFakeInstall(t *testing.T) *fakeInstall {
	// The updater resolves symlinks, such as /var on macOS, before comparing paths.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeInstall{t: t, dir: dir, payload: "0.2.0"}
	if runtime.GOOS == "darwin" {
		f.target = filepath.Join(f.dir, "Orchestra.app")
		f.executable = filepath.Join(f.target, "Contents", "MacOS", "orchestra")
	} else {
		f.target = filepath.Join(f.dir, "orchestra")
		f.executable = f.target
	}
	f.write(f.target, "0.1.0")
	return f
}

func (f *fakeInstall) write(path, content string) {
	file := path
	if runtime.GOOS == "darwin" {
		file = filepath.Join(path, "Contents", "MacOS", "orchestra")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeInstall) installed() string {
	data, err := os.ReadFile(f.executable)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *fakeInstall) stages() []string {
	stages, _ := filepath.Glob(filepath.Join(f.dir, stagePrefix+"*"))
	return stages
}

func (f *fakeInstall) updater() *Updater {
	source := func(context.Context) ([]UpdateRelease, error) {
		return []UpdateRelease{{Version: "0.2.0"}}, nil
	}
	download := func(ctx context.Context, version string, assets []string, dir string) error {
		if f.download != nil {
			if err := f.download(ctx); err != nil {
				return err
			}
		}
		if len(assets) != 2 || assets[0] != updateAsset(version) || assets[1] != "SHA256SUMS" {
			f.t.Fatalf("downloaded %v", assets)
		}
		sum := sha256.Sum256([]byte(f.payload))
		sums := hex.EncodeToString(sum[:]) + "  " + assets[0] + "\n"
		if f.sums != "" {
			sums = f.sums
		}
		if err := os.WriteFile(filepath.Join(dir, assets[0]), []byte(f.payload), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums), 0600)
	}
	u := NewUpdater(UpdaterOptions{Current: "0.1.0", Executable: f.executable, Source: source, Download: download, Preferences: func() Preferences { return Preferences{} }})
	u.unpack = func(_ context.Context, archive, dir, _ string) (string, error) {
		data, err := os.ReadFile(archive)
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, "unpacked")
		f.write(path, string(data))
		return path, nil
	}
	u.verify = func(_ context.Context, path, current, version string) error {
		if current != f.target || version != "0.2.0" {
			f.t.Fatalf("verified %s against %s for %s", path, current, version)
		}
		return f.verifyErr
	}
	u.relaunch = func(pid int, path string) *exec.Cmd {
		if pid != os.Getpid() {
			f.t.Fatalf("relaunch waits for pid %d", pid)
		}
		f.relaunched = append(f.relaunched, path)
		return exec.Command("true")
	}
	if !u.Status().Installable {
		if runtime.GOOS == "linux" && runtime.GOARCH != "amd64" {
			f.t.Skip("releases include Linux builds for x86_64 only")
		}
		f.t.Fatalf("fixture is not installable: %+v", u.Status())
	}
	u.Check(context.Background(), true)
	return u
}

func TestUpdateInstallAndRestart(t *testing.T) {
	f := newFakeInstall(t)
	u := f.updater()
	status := u.Install(context.Background(), "0.2.0", true)
	if status.Staged != "0.2.0" || status.Preparing || status.InstallError != "" {
		t.Fatalf("install status: %+v", status)
	}
	if f.installed() != "0.1.0" || len(f.stages()) != 1 {
		t.Fatal("staging changed the installed app or left no stage beside it")
	}
	if installed, err := u.ApplyStaged(); err != nil || installed != "0.2.0" {
		t.Fatalf("installed %q, %v", installed, err)
	}
	if f.installed() != "0.2.0" || len(f.stages()) != 0 {
		t.Fatalf("installed %q with stages %v", f.installed(), f.stages())
	}
	if len(f.relaunched) != 1 || f.relaunched[0] != f.target {
		t.Fatalf("relaunched %v", f.relaunched)
	}
	if installed, err := u.ApplyStaged(); err != nil || installed != "" || len(f.relaunched) != 1 {
		t.Fatal("a second quit applied the update again")
	}
}

func TestUpdateInstallWhenQuitting(t *testing.T) {
	f := newFakeInstall(t)
	u := f.updater()
	if status := u.Install(context.Background(), "0.2.0", false); status.Staged != "0.2.0" {
		t.Fatalf("install status: %+v", status)
	}
	if _, err := u.ApplyStaged(); err != nil {
		t.Fatal(err)
	}
	if f.installed() != "0.2.0" || len(f.relaunched) != 0 {
		t.Fatalf("installed %q, relaunched %v", f.installed(), f.relaunched)
	}
}

func TestUpdateInstallRejectsBadDownloads(t *testing.T) {
	for name, setup := range map[string]func(*fakeInstall){
		"checksum":  func(f *fakeInstall) { f.sums = strings.Repeat("0", 64) + "  " + updateAsset("0.2.0") + "\n" },
		"unlisted":  func(f *fakeInstall) { f.sums = strings.Repeat("0", 64) + "  other.zip\n" },
		"signature": func(f *fakeInstall) { f.verifyErr = errors.New("The update is not notarized by Apple.") },
		"network":   func(f *fakeInstall) { f.download = func(context.Context) error { return errors.New("offline") } },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeInstall(t)
			setup(f)
			u := f.updater()
			status := u.Install(context.Background(), "0.2.0", true)
			if status.Staged != "" || status.Preparing || !strings.HasPrefix(status.InstallError, "Could not install the update: ") {
				t.Fatalf("install status: %+v", status)
			}
			if _, err := u.ApplyStaged(); err != nil {
				t.Fatal(err)
			}
			if f.installed() != "0.1.0" || len(f.stages()) != 0 || len(f.relaunched) != 0 {
				t.Fatalf("a failed install changed the app: %q %v %v", f.installed(), f.stages(), f.relaunched)
			}
		})
	}
}

func TestUpdateInstallOnlyTheAvailableVersion(t *testing.T) {
	f := newFakeInstall(t)
	u := f.updater()
	if status := u.Install(context.Background(), "9.9.9", true); status.Staged != "" || status.InstallError == "" {
		t.Fatalf("installed a version that is not available: %+v", status)
	}
}

func TestUpdateInstallCancel(t *testing.T) {
	f := newFakeInstall(t)
	started := make(chan struct{})
	f.download = func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	u := f.updater()
	done := make(chan UpdateStatus)
	go func() { done <- u.Install(context.Background(), "0.2.0", true) }()
	<-started
	if !u.Status().Preparing {
		t.Fatal("status does not show the download")
	}
	u.CancelInstall()
	if status := <-done; status.Staged != "" || status.Preparing || status.InstallError != "" {
		t.Fatalf("cancelled install: %+v", status)
	}
	if len(f.stages()) != 0 {
		t.Fatal("cancelled install left a stage")
	}
}

func TestUpdateInstallUnavailable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to any folder")
	}
	f := newFakeInstall(t)
	if err := os.Chmod(f.dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(f.dir, 0700)
	u := NewUpdater(UpdaterOptions{Current: "0.1.0", Executable: f.executable, Source: func(context.Context) ([]UpdateRelease, error) { return nil, nil }, Download: func(context.Context, string, []string, string) error {
		t.Fatal("downloaded without write access")
		return nil
	}, Preferences: func() Preferences { return Preferences{} }})
	status := u.Status()
	if status.Installable || !strings.Contains(status.InstallNote, "cannot replace itself") {
		t.Fatalf("status: %+v", status)
	}
	if status = u.Install(context.Background(), "0.2.0", true); status.InstallError != status.InstallNote {
		t.Fatalf("install status: %+v", status)
	}
	// Development builds and missing downloaders never offer installation.
	if NewUpdater(UpdaterOptions{Current: "dev", Executable: f.executable, Download: GitHubDownload}).Status().Installable {
		t.Fatal("a development build is installable")
	}
}

func TestUpdaterRemovesStaleStages(t *testing.T) {
	f := newFakeInstall(t)
	stale := filepath.Join(f.dir, stagePrefix+"interrupted")
	if err := os.MkdirAll(stale, 0700); err != nil {
		t.Fatal(err)
	}
	f.updater()
	if len(f.stages()) != 0 {
		t.Fatal("stale stage was kept")
	}
}

func TestSwapInstalledRestoresOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Orchestra.app")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := swapInstalled(target, filepath.Join(dir, "missing"), filepath.Join(dir, "previous")); err == nil {
		t.Fatal("swapped in a missing update")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("the previous app was not restored")
	}
}
