package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Hook commands only observe Orchestra-owned sessions and never return tool
// approval decisions, inject prompts, or change a provider's permissions.
func agentStatusCommand(agent, state string, event ...string) string {
	name := ""
	if len(event) > 0 {
		name = " " + shellQuote(event[0])
	}
	return "if [ \"$ORCHESTRA_AGENT_KIND\" = " + shellQuote(agent) + " ]; then \"$ORCHESTRA_CLI_DIRECTORY/orchestra\" agent-status " + state + name + " >/dev/null 2>&1 || :; else /bin/cat >/dev/null; fi"
}

func writeAgentJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0600)
}

// Managed plugin directories are separate from user-authored plugins. A name
// collision is an error rather than permission to replace someone else's files.
func managedAgentDirectory(dir string) error {
	marker := filepath.Join(dir, ".orchestra-managed")
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Orchestra plugin path is not a directory.")
		}
		if _, err = os.Stat(marker); err != nil {
			return fmt.Errorf("%s already exists and is not managed by Orchestra", dir)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return atomicWrite(marker, []byte("Orchestra session status\n"), 0600)
}

func agentConfigFolder(env map[string]string, variable, fallback string) (string, error) {
	if folder := env[variable]; folder != "" {
		return folder, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback), nil
}

func groupedStatusHooks(agent string, events map[string]string) map[string]any {
	hooks := map[string]any{}
	for event, state := range events {
		hooks[event] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": agentStatusCommand(agent, state, event)}}}}
	}
	return hooks
}

// reportAgentStatus accepts the providers' documented hook payloads. Child-agent
// events cannot replace the root conversation or mark its turn as complete.
func reportAgentStatus(in io.Reader, state, statusFile, idFile string, hookEvent ...string) error {
	defer io.Copy(io.Discard, in)
	if state != "working" && state != "waiting" {
		return errors.New("Invalid agent activity.")
	}
	var event struct {
		SessionID            string `json:"session_id"`
		SessionIDCamel       string `json:"sessionId"`
		ConversationID       string `json:"conversation_id"`
		AgentID              string `json:"agent_id"`
		ParentSessionID      string `json:"parent_session_id"`
		ParentConversationID string `json:"parent_conversation_id"`
		Event                string `json:"hook_event_name"`
		Source               string `json:"source"`
		EventName            string `json:"event"`
		Notification         string `json:"notification_type"`
		Recoverable          bool   `json:"recoverable"`
		SessionContext       struct {
			RootSessionID string `json:"rootSessionId"`
		} `json:"sessionContext"`
	}
	err := json.NewDecoder(io.LimitReader(in, 4*1024*1024)).Decode(&event)
	if err != nil && err != io.EOF {
		return err
	}
	if event.AgentID != "" || event.ParentSessionID != "" || event.ParentConversationID != "" {
		return nil
	}
	name := event.Event
	if len(hookEvent) > 0 {
		name = hookEvent[0]
	}
	if name == "" {
		name = event.EventName
	}
	if strings.HasPrefix(strings.ToLower(name), "subagent") {
		return nil
	}
	// Copilot emits errors during automatic retries too; those do not need input.
	if strings.EqualFold(name, "ErrorOccurred") && event.Recoverable {
		return nil
	}
	if strings.EqualFold(name, "Notification") && event.Notification != "permission_prompt" && event.Notification != "idle_prompt" && event.Notification != "elicitation_dialog" {
		return nil
	}
	id := event.SessionID
	if id == "" {
		id = event.SessionIDCamel
	}
	if id == "" {
		id = event.ConversationID
	}
	if id == "" {
		id = event.SessionContext.RootSessionID
	}
	if len(id) > 1024 || strings.IndexFunc(id, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return errors.New("Invalid conversation ID.")
	}
	rootChanged := false
	if id != "" && idFile != "" {
		previous, _ := os.ReadFile(idFile)
		rootChanged = len(previous) > 0 && string(previous) != id
		if len(previous) > 0 && string(previous) != id && !(strings.EqualFold(name, "SessionStart") && (event.Source == "new" || event.Source == "clear")) {
			return nil
		}
		if err = atomicWrite(idFile, []byte(id), 0600); err != nil {
			return err
		}
	}
	if statusFile == "" {
		return errors.New("Missing activity file.")
	}
	if strings.EqualFold(name, "SessionStart") && !rootChanged {
		if _, err := os.Stat(statusFile); err == nil {
			return nil
		}
	}
	return atomicWrite(statusFile, []byte(state), 0600)
}
