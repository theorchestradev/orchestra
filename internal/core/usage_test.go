package core

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageNormalization(t *testing.T) {
	windows, credits, err := parseCodexUsage([]byte(`{"rateLimits":{"primary":{"usedPercent":90}},"rateLimitsByLimitId":{"codex":{"primary":null,"secondary":{"usedPercent":3,"windowDurationMins":10080,"resetsAt":1800000000}},"spark":{"limitName":"Spark","primary":{"usedPercent":120,"windowDurationMins":300},"secondary":{"usedPercent":null}}},"rateLimitResetCredits":{"availableCount":2}}`))
	if err != nil || len(windows) != 2 || windows[0].Remaining != 97 || windows[0].Label != "Weekly" || windows[1].Remaining != 0 || credits == nil || *credits != 2 {
		t.Fatalf("unexpected Codex normalization: %+v %v %v", windows, credits, err)
	}
	windows, _, err = parseCodexUsage([]byte(`{"rateLimits":{"primary":{"usedPercent":0,"windowDurationMins":15}}}`))
	if err != nil || len(windows) != 1 || windows[0].Remaining != 100 || windows[0].Label != "15m window" {
		t.Fatalf("legacy Codex: %+v %v", windows, err)
	}
	windows, err = parseClaudeUsage([]byte(`{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":0,"resets_at":null},"seven_day":{"utilization":4,"resets_at":"2026-10-09T12:00:00Z"},"model_scoped":[{"display_name":"Fable","utilization":0,"resets_at":"2026-10-09T12:00:00Z"},{"display_name":"Missing","utilization":null}]}}`))
	if err != nil || len(windows) != 3 || windows[0].Remaining != 100 || windows[0].ResetsAt != 0 || windows[1].Remaining != 96 || windows[2].Label != "Weekly · Fable" {
		t.Fatalf("unexpected Claude normalization: %+v %v", windows, err)
	}
	if windows, err = parseClaudeUsage([]byte(`{"rate_limits_available":false}`)); err != nil || len(windows) != 0 {
		t.Fatal("unsupported accounts must not fabricate allowance")
	}
	if _, err = parseClaudeUsage([]byte(`{"rate_limits_available":true,"rate_limits":null}`)); err == nil {
		t.Fatal("missing response must be unavailable")
	}
	if _, _, err = parseCodexUsage([]byte(`not json`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestUsageControlQueries(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestUsageControlHelper", "--", agent)
			cmd.Env = append(os.Environ(), "ORCHESTRA_USAGE_HELPER=1")
			data, err := queryUsage(cmd, agent)
			if err != nil || !strings.Contains(string(data), "usageFixture") {
				t.Fatalf("control query failed: %s %v", data, err)
			}
			if cmd.ProcessState == nil {
				t.Fatal("probe was not reaped")
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestUsageControlHelper", "--", "hang")
	cmd.Env = append(os.Environ(), "ORCHESTRA_USAGE_HELPER=1")
	if _, err := queryUsage(cmd, "codex"); err == nil || ctx.Err() == nil || cmd.ProcessState == nil {
		t.Fatal("hung CLI must be stopped and reaped")
	}
}

func TestUsageInstalledCLIIntegration(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PATH", directory)
	t.Setenv("ORCHESTRA_USAGE_HELPER", "1")
	for _, agent := range []string{"codex", "claude"} {
		wrapper := "#!/bin/sh\nexec " + shellQuote(os.Args[0]) + " -test.run=TestUsageControlHelper -- \"$@\" " + agent + "\n"
		if err := os.WriteFile(filepath.Join(directory, agent), []byte(wrapper), 0700); err != nil {
			t.Fatal(err)
		}
	}
	results := ReadAgentUsage(context.Background(), nil)
	if len(results) != 2 {
		t.Fatal(results)
	}
	for _, result := range results {
		if result.Message != "" || len(result.Windows) != 1 || result.Windows[0].Remaining != 97 {
			t.Fatalf("installed CLI query: %+v", result)
		}
	}
}

// Isolated protocol peer: rejects any user message, turn, or unexpected method.
func TestUsageControlHelper(t *testing.T) {
	if os.Getenv("ORCHESTRA_USAGE_HELPER") != "1" {
		return
	}
	agent := os.Args[len(os.Args)-1]
	args := strings.Join(os.Args, " ")
	if strings.Contains(args, "--print") && (!strings.Contains(args, "--no-session-persistence") || !strings.Contains(args, "--strict-mcp-config") || !strings.Contains(args, "disableAllHooks") || os.Getenv("ENABLE_CLAUDEAI_MCP_SERVERS") != "false") {
		os.Exit(5)
	}
	if agent == "hang" {
		time.Sleep(time.Minute)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	stage := 0
	for scanner.Scan() {
		var m map[string]any
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			os.Exit(2)
		}
		if agent == "codex" {
			method, _ := m["method"].(string)
			switch {
			case stage == 0 && method == "initialize":
				stage = 1
				_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{}})
			case stage == 1 && method == "initialized":
				stage = 2
			case stage == 2 && method == "account/rateLimits/read":
				_ = encoder.Encode(map[string]any{"id": 2, "result": map[string]any{"usageFixture": true, "rateLimits": map[string]any{"secondary": map[string]any{"usedPercent": 3, "windowDurationMins": 10080}}}})
				stage = 3
			default:
				os.Exit(3)
			}
		} else {
			r, _ := m["request"].(map[string]any)
			if m["type"] != "control_request" || (stage == 0 && r["subtype"] != "initialize") || (stage == 1 && (r["subtype"] != "get_usage" || r["skip_behaviors"] != true)) || stage > 1 {
				os.Exit(4)
			}
			result := map[string]any{}
			if stage == 1 {
				result["usageFixture"] = true
				result["rate_limits_available"] = true
				result["rate_limits"] = map[string]any{"seven_day": map[string]any{"utilization": 3}}
			}
			_ = encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"request_id": m["request_id"], "subtype": "success", "response": result}})
			stage++
		}
	}
	os.Exit(0)
}
