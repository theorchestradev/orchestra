package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

type UsageWindow struct {
	Label     string  `json:"label"`
	Remaining float64 `json:"remaining"`
	ResetsAt  int64   `json:"resetsAt,omitempty"`
}

type AgentUsage struct {
	Agent        string        `json:"agent"`
	Windows      []UsageWindow `json:"windows"`
	ResetCredits *int          `json:"resetCredits,omitempty"`
	CheckedAt    int64         `json:"checkedAt"`
	Message      string        `json:"message,omitempty"`
}

// These are account-control queries, never agent turns. No credentials cross
// the Wails bridge; authentication and token refresh stay with the installed CLI.
func ReadAgentUsage(ctx context.Context) []AgentUsage {
	results := make([]AgentUsage, 2)
	var wg sync.WaitGroup
	for i, agent := range []string{"codex", "claude"} {
		wg.Go(func() { results[i] = readAgentUsage(ctx, agent) })
	}
	wg.Wait()
	return results
}

func readAgentUsage(parent context.Context, agent string) AgentUsage {
	result := AgentUsage{Agent: agent, Windows: []UsageWindow{}, CheckedAt: time.Now().Unix()}
	env := environmentMap()
	home, _ := os.UserHomeDir()
	searchPath := env["PATH"] + ":" + filepath.Join(home, ".local", "bin") + ":/opt/homebrew/bin:/usr/local/bin"
	var executable string
	for _, directory := range filepath.SplitList(searchPath) {
		if !filepath.IsAbs(directory) {
			continue
		}
		if path, err := exec.LookPath(filepath.Join(directory, agent)); err == nil {
			executable = path
			break
		}
	}
	if executable == "" {
		result.Message = "Install and sign in to the CLI to see subscription limits."
		return result
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	// A neutral working directory prevents project configuration from loading.
	directory, err := os.MkdirTemp("", "orchestra-usage-")
	if err != nil {
		result.Message = "Could not prepare the usage check."
		return result
	}
	defer os.RemoveAll(directory)
	args := []string{"app-server"}
	if agent == "claude" {
		args = []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--setting-sources=", "--settings", `{"disableAllHooks":true}`, "--tools", "", "--mcp-config", `{"mcpServers":{}}`, "--strict-mcp-config"}
		delete(env, "CLAUDECODE")
		env["CLAUDE_CODE_ENTRYPOINT"] = "sdk-ts"
		env["ENABLE_CLAUDEAI_MCP_SERVERS"] = "false"
		env["CLAUDE_CODE_AUTO_CONNECT_IDE"] = "0"
		env["CLAUDE_CODE_IDE_SKIP_AUTO_INSTALL"] = "1"
		env["DISABLE_AUTOUPDATER"] = "1"
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir, cmd.Env = directory, environmentList(env)
	data, err := queryUsage(cmd, agent)
	if err != nil {
		result.Message = "Could not read limits. Check your CLI sign-in and version, then refresh."
		if ctx.Err() != nil {
			result.Message = "The usage check timed out. Try refreshing again."
		}
		return result
	}
	if agent == "codex" {
		result.Windows, result.ResetCredits, err = parseCodexUsage(data)
	} else {
		result.Windows, err = parseClaudeUsage(data)
	}
	if err != nil {
		result.Message = "The CLI returned an unsupported usage response. Try updating it."
	} else if len(result.Windows) == 0 {
		result.Message = "This account does not report subscription limits."
	}
	return result
}

func queryUsage(cmd *exec.Cmd, agent string) (json.RawMessage, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard // Never return CLI diagnostics or account details to the UI.
	if err = cmd.Start(); err != nil {
		input.Close()
		return nil, err
	}
	defer func() { input.Close(); _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() }()
	send := func(value any) error { return json.NewEncoder(input).Encode(value) }
	request := func(id int, method string, params any) error {
		if agent == "codex" {
			return send(map[string]any{"id": id, "method": method, "params": params})
		}
		return send(map[string]any{"type": "control_request", "request_id": fmt.Sprint(id), "request": params})
	}
	init := any(map[string]any{"clientInfo": map[string]string{"name": "orchestra_usage", "title": "Orchestra", "version": "1.0"}})
	if agent == "claude" {
		init = map[string]any{"subtype": "initialize", "hooks": map[string]any{}, "sdkMcpServers": []string{}}
	}
	if err := request(1, "initialize", init); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	initialized := false
	for scanner.Scan() {
		var message struct {
			ID       int             `json:"id"`
			Type     string          `json:"type"`
			Result   json.RawMessage `json:"result"`
			Error    json.RawMessage `json:"error"`
			Response struct {
				ID      string          `json:"request_id"`
				Subtype string          `json:"subtype"`
				Result  json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		id, data, failed := message.ID, message.Result, len(message.Error) > 0 && string(message.Error) != "null"
		if agent == "claude" {
			if message.Type != "control_response" {
				continue
			}
			id = 0
			if message.Response.ID == "1" {
				id = 1
			} else if message.Response.ID == "2" {
				id = 2
			}
			data, failed = message.Response.Result, message.Response.Subtype != "success"
		}
		if id != 1 && id != 2 {
			continue
		}
		if failed {
			return nil, errors.New("usage control request failed")
		}
		if id == 1 && !initialized {
			initialized = true
			if agent == "codex" {
				if err := send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
					return nil, err
				}
			}
			params := any(map[string]any{})
			if agent == "claude" {
				params = map[string]any{"subtype": "get_usage", "skip_behaviors": true}
			}
			if err := request(2, "account/rateLimits/read", params); err != nil {
				return nil, err
			}
		} else if id == 2 && initialized {
			return data, nil
		}
	}
	return nil, errors.New("CLI closed before reporting usage")
}

func remaining(used float64) float64 { return math.Max(0, math.Min(100, 100-used)) }

func parseCodexUsage(data []byte) ([]UsageWindow, *int, error) {
	type window struct {
		Used     *float64 `json:"usedPercent"`
		Duration int      `json:"windowDurationMins"`
		Reset    int64    `json:"resetsAt"`
	}
	type bucket struct {
		Name      string  `json:"limitName"`
		Primary   *window `json:"primary"`
		Secondary *window `json:"secondary"`
	}
	var response struct {
		Limits  *bucket           `json:"rateLimits"`
		Buckets map[string]bucket `json:"rateLimitsByLimitId"`
		Credits *struct {
			Count *int `json:"availableCount"`
		} `json:"rateLimitResetCredits"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, nil, err
	}
	keys := make([]string, 0, len(response.Buckets))
	for key := range response.Buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 && response.Limits != nil {
		response.Buckets = map[string]bucket{"codex": *response.Limits}
		keys = []string{"codex"}
	}
	rows := []UsageWindow{}
	for _, key := range keys {
		b := response.Buckets[key]
		for _, w := range []*window{b.Primary, b.Secondary} {
			if w == nil || w.Used == nil {
				continue
			}
			label := "Session"
			switch w.Duration {
			case 10080:
				label = "Weekly"
			case 1440:
				label = "Daily"
			case 300:
				label = "Session · 5h"
			default:
				if w.Duration > 0 {
					if w.Duration%60 == 0 {
						label = fmt.Sprintf("%dh window", w.Duration/60)
					} else {
						label = fmt.Sprintf("%dm window", w.Duration)
					}
				}
			}
			if key != "codex" {
				name := b.Name
				if name == "" {
					name = key
				}
				label += " · " + name
			}
			rows = append(rows, UsageWindow{label, remaining(*w.Used), w.Reset})
		}
	}
	var credits *int
	if response.Credits != nil {
		credits = response.Credits.Count
	}
	return rows, credits, nil
}

func parseClaudeUsage(data []byte) ([]UsageWindow, error) {
	type window struct {
		Used  *float64 `json:"utilization"`
		Reset string   `json:"resets_at"`
		Name  string   `json:"display_name"`
	}
	var response struct {
		Available bool `json:"rate_limits_available"`
		Limits    *struct {
			Session *window  `json:"five_hour"`
			Weekly  *window  `json:"seven_day"`
			Models  []window `json:"model_scoped"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	rows := []UsageWindow{}
	if !response.Available {
		return rows, nil
	}
	if response.Limits == nil {
		return nil, errors.New("missing rate limits")
	}
	add := func(label string, w *window) {
		if w == nil || w.Used == nil {
			return
		}
		reset, _ := time.Parse(time.RFC3339, w.Reset)
		var epoch int64
		if !reset.IsZero() {
			epoch = reset.Unix()
		}
		rows = append(rows, UsageWindow{label, remaining(*w.Used), epoch})
	}
	add("Session · 5h", response.Limits.Session)
	add("Weekly", response.Limits.Weekly)
	for _, w := range response.Limits.Models {
		label := "Weekly"
		if w.Name != "" {
			label += " · " + w.Name
		}
		add(label, &w)
	}
	return rows, nil
}
