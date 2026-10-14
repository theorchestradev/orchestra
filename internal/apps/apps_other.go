//go:build !darwin

package apps

import (
	"errors"
	"os/exec"
)

const fileManagerName = "File Manager"

// Installed lists the known apps whose executable is on PATH. Linux apps have no icon here.
func Installed() []App {
	result := []App{}
	for _, app := range knownApps {
		if executable(app) != "" {
			result = append(result, App{ID: app.id, Name: app.name, Kind: app.kind})
		}
	}
	return result
}

func executable(app known) string {
	for _, name := range app.executables {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// Open starts the app on folder without waiting for it to quit.
func Open(id, folder string) error {
	app, ok := find(id)
	if !ok {
		return errors.New("Unknown app.")
	}
	path := executable(app)
	if path == "" {
		return errors.New(app.name + " is not installed.")
	}
	cmd := exec.Command(path, folder)
	if app.kind == Terminal {
		cmd = exec.Command(path)
	}
	cmd.Dir = folder
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
