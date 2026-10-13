package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Vibe logs its sessions to a per-session folder, where CaptureID reads the conversation ID.
var vibeAgent = &agentSpec{
	ID: "vibe", Name: "Mistral Vibe", Executable: "vibe", SkillKey: "mistral-vibe",
	Command: func(l launch) string { return l.command("vibe", "--resume", "--") },
	Prepare: func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error {
		t.agentDirectory = filepath.Join(filepath.Dir(s.path), "AgentSessions", "Vibe", sess.ID)
		if err := os.MkdirAll(t.agentDirectory, 0700); err != nil {
			return err
		}
		config := map[string]any{}
		if env["VIBE_SESSION_LOGGING"] != "" {
			if err := json.Unmarshal([]byte(env["VIBE_SESSION_LOGGING"]), &config); err != nil {
				return err
			}
		}
		config["enabled"] = true
		config["save_dir"] = t.agentDirectory
		data, _ := json.Marshal(config)
		env["VIBE_SESSION_LOGGING"] = string(data)
		return nil
	},
	CaptureID: captureVibeID,
}

// captureVibeID returns the session ID of the latest log written since launch.
func captureVibeID(t *TerminalSession) string {
	if t.agentDirectory == "" {
		return ""
	}
	entries, _ := os.ReadDir(t.agentDirectory)
	latest, id := t.launched, ""
	for _, entry := range entries {
		path := filepath.Join(t.agentDirectory, entry.Name())
		if entry.IsDir() {
			path = filepath.Join(path, "meta.json")
		}
		info, err := os.Stat(path)
		if err != nil || info.ModTime().Before(latest) || info.Size() > 1024*1024 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta map[string]any
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		if nested, ok := meta["metadata"].(map[string]any); ok {
			meta = nested
		}
		if value, _ := meta["session_id"].(string); value != "" {
			latest, id = info.ModTime(), value
		}
	}
	return id
}
