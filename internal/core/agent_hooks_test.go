package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAgentStatusEventsStayInTheirRootConversation(t *testing.T) {
	dir := t.TempDir()
	status, id := filepath.Join(dir, "status"), filepath.Join(dir, "id")
	cases := []struct{ payload, state, event, wantID, wantState string }{
		{`{"session_id":"root"}`, "waiting", "SessionStart", "root", "waiting"},
		{`{"sessionId":"root"}`, "working", "userPromptSubmitted", "root", "working"},
		{`{"conversation_id":"root"}`, "waiting", "sessionStart", "root", "working"},
		{`{"sessionContext":{"rootSessionId":"root"}}`, "working", "TaskStart", "root", "working"},
		{`{"session_id":"child","agent_id":"a"}`, "waiting", "Stop", "root", "working"},
		{`{"session_id":"root","agent_id":"a"}`, "waiting", "PermissionRequest", "root", "working"},
		{`{"session_id":"child","parent_session_id":"root"}`, "waiting", "SessionStart", "root", "working"},
		{`{"session_id":"child"}`, "waiting", "SessionStart", "root", "working"},
		{`{"session_id":"child"}`, "waiting", "Stop", "root", "working"},
		{`{"session_id":"root","notification_type":"auth_success"}`, "waiting", "Notification", "root", "working"},
		{`{"session_id":"root","notification_type":"permission_prompt"}`, "waiting", "Notification", "root", "waiting"},
		{`{"sessionId":"root"}`, "working", "userPromptSubmitted", "root", "working"},
		{`{"session_id":"root","notification_type":"elicitation_dialog"}`, "waiting", "Notification", "root", "waiting"},
		{`{"conversation_id":"root"}`, "working", "postToolUse", "root", "working"},
		{`{"sessionId":"root"}`, "waiting", "agentStop", "root", "waiting"},
		{`{"sessionId":"root"}`, "working", "userPromptSubmitted", "root", "working"},
		{`{"sessionId":"root","recoverable":true}`, "waiting", "errorOccurred", "root", "working"},
		{`{"sessionId":"root","recoverable":false}`, "waiting", "errorOccurred", "root", "waiting"},
		{`{"sessionId":"new","source":"new"}`, "waiting", "sessionStart", "new", "waiting"},
	}
	for _, c := range cases {
		if err := reportAgentStatus(strings.NewReader(c.payload), c.state, status, id, c.event); err != nil {
			t.Fatal(err)
		}
		gotID, _ := os.ReadFile(id)
		gotStatus, _ := os.ReadFile(status)
		if string(gotID) != c.wantID || string(gotStatus) != c.wantState {
			t.Fatalf("%s %s: id=%q state=%q", c.event, c.payload, gotID, gotStatus)
		}
	}
	if err := reportAgentStatus(strings.NewReader(`{}`), "invalid", status, id); err == nil {
		t.Fatal("accepted invalid state")
	}
}

// Run every generated command against the real private CLI, with a temporary HOME.
// No provider binary, credentials, or user settings are used.
func TestAgentHookCommandsAreSilentAndSessionScoped(t *testing.T) {
	for _, agent := range []string{"cursor", "copilot", "droid", "qwen", "goose", "kiro", "kimi"} {
		t.Run(agent, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := t.TempDir()
			exe, _ := os.Executable()
			if err := os.WriteFile(filepath.Join(dir, "orchestra"), []byte("#!/bin/sh\nexec "+shellQuote(exe)+" --orchestra-cli \"$@\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			sess := &Session{ID: "session", Agent: agent}
			term := &TerminalSession{idFile: filepath.Join(dir, "id")}
			env := map[string]string{"PATH": dir + ":/usr/bin:/bin", "HOME": home, "ORCHESTRA_AGENT_KIND": agent, "ORCHESTRA_CLI_DIRECTORY": dir, "ORCHESTRA_AGENT_ID_FILE": term.idFile, "ORCHESTRA_STATUS_FILE": filepath.Join(dir, "status")}
			store := &Store{path: filepath.Join(home, "state.json"), runtimeDir: dir}
			if err := agentSpecs[agent].Prepare(store, sess, term, env); err != nil {
				t.Fatal(err)
			}
			commands := []string{}
			for _, root := range []string{home, dir} {
				filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() {
						return nil
					}
					data, _ := os.ReadFile(path)
					if filepath.Ext(path) == ".json" {
						var value any
						if json.Unmarshal(data, &value) == nil {
							collectHookCommands(value, &commands)
						}
					}
					if filepath.Ext(path) == ".toml" {
						for _, line := range strings.Split(string(data), "\n") {
							if v, ok := strings.CutPrefix(line, "command = "); ok {
								cmd, err := strconv.Unquote(v)
								if err != nil {
									t.Fatal(err)
								}
								commands = append(commands, cmd)
							}
						}
					}
					return nil
				})
			}
			if len(commands) == 0 {
				t.Fatal("no hooks generated")
			}
			rootID := "root"
			if saved, err := os.ReadFile(term.idFile); err == nil && len(saved) > 0 {
				rootID = string(saved)
			}
			payload, _ := json.Marshal(map[string]string{"session_id": rootID, "sessionId": rootID, "conversation_id": rootID, "notification_type": "permission_prompt"})
			for _, command := range commands {
				cmd := exec.Command("/bin/sh", "-c", command)
				cmd.Env = environmentList(env)
				cmd.Stdin = strings.NewReader(string(payload))
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v %s", command, err, out)
				}
				if len(out) > 0 && string(out) != `{"continue":true}` {
					t.Fatalf("hook returned a decision or context: %q", out)
				}
				state, _ := os.ReadFile(env["ORCHESTRA_STATUS_FILE"])
				if len(state) == 0 {
					t.Fatal("hook did not publish status")
				}
				_ = os.Remove(env["ORCHESTRA_STATUS_FILE"])
				env["ORCHESTRA_AGENT_KIND"] = "unrelated"
				cmd = exec.Command("/bin/sh", "-c", command)
				cmd.Env = environmentList(env)
				cmd.Stdin = strings.NewReader(`{}`)
				if out, err = cmd.CombinedOutput(); err != nil {
					t.Fatalf("outside session: %v %s", err, out)
				}
				if _, err = os.Stat(env["ORCHESTRA_STATUS_FILE"]); !os.IsNotExist(err) {
					t.Fatal("hook changed another session")
				}
				env["ORCHESTRA_AGENT_KIND"] = agent
			}
		})
	}
}
func collectHookCommands(value any, out *[]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "command" || key == "bash" {
				if command, ok := item.(string); ok {
					*out = append(*out, command)
				}
			} else {
				collectHookCommands(item, out)
			}
		}
	case []any:
		for _, item := range v {
			collectHookCommands(item, out)
		}
	}
}

func TestKimiHooksPreserveUserConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "default_model = \"user-model\"\n[[hooks]]\nevent = \"Stop\"\ncommand = \"echo user-hook\"\n[models.user-model]\nmodel = \"example\"\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := installKimiStatusHooks(path); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), original) || strings.Count(string(data), "Orchestra session status hooks BEGIN") != 1 {
		t.Fatal("changed user config or duplicated hooks")
	}
	inline := "hooks = []\n"
	_ = os.WriteFile(path, []byte(inline), 0600)
	if installKimiStatusHooks(path) == nil {
		t.Fatal("accepted conflicting inline hooks")
	}
	data, _ = os.ReadFile(path)
	if string(data) != inline {
		t.Fatal("changed inline user hooks")
	}
}

func TestAddedAgentModelOptions(t *testing.T) {
	for _, id := range []string{"copilot", "qwen", "goose", "kiro", "droid", "kimi"} {
		options, err := LaunchOptions(context.Background(), id, nil)
		if err != nil || !options.CustomModel {
			t.Fatalf("%s: %+v %v", id, options, err)
		}
		if err = validateLaunch(id, "provider/model", ""); err != nil {
			t.Fatal(err)
		}
	}
	models, err := parseCursorModels([]byte("banner\nAvailable models\n\x1b[32mclaude-opus-4-8[effort=high]\x1b[0m - Claude Opus (current)\nTip: use --model\n"))
	if err != nil || len(models) != 1 || models[0].ID != "claude-opus-4-8[effort=high]" {
		t.Fatalf("%+v %v", models, err)
	}
}

func TestCursorProfileAuthenticationAndSkillCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := filepath.Join(home, "profile")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	profile := &Profile{ID: "p", Name: "Work", Folders: map[string]string{"cursor": folder}}
	env := profileFolders(profile)
	if env["CURSOR_CONFIG_DIR"] != folder || env["CURSOR_DATA_DIR"] != folder {
		t.Fatalf("Cursor profile not isolated: %+v", env)
	}
	authenticated := true
	scripts := fakeSkillShell(t, func(script string) ([]byte, int, error) {
		if !strings.Contains(script, "cursor-agent") {
			return nil, 1, nil
		}
		if authenticated {
			return []byte(`{"isAuthenticated":true}`), 0, nil
		}
		return []byte(`{"isAuthenticated":false}`), 0, nil
	})
	result := ProfileLogins(context.Background(), profile)
	if len(*scripts) != 1 || !result[len(result)-1].LoggedIn || !strings.Contains((*scripts)[0], "CURSOR_DATA_DIR=") {
		t.Fatalf("Cursor sign-in: %+v %q", result, *scripts)
	}
	authenticated = false
	result = ProfileLogins(context.Background(), profile)
	if result[len(result)-1].LoggedIn {
		t.Fatal("exit zero was mistaken for authentication")
	}
	source := filepath.Join(home, ".cursor", "skills", "orchestra", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("orchestra guide"), 0600); err != nil {
		t.Fatal(err)
	}
	*scripts = nil
	if _, ran, err := installProfileSkill(context.Background(), profile, []string{"cursor"}); err != nil || !ran {
		t.Fatalf("profile skill: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(folder, "skills", "orchestra", "SKILL.md"))
	if string(data) != "orchestra guide" || len(*scripts) != 0 {
		t.Fatal("profile skill did not use the installed public stub")
	}
}

func TestProfileUpgradeKeepsExistingAccount(t *testing.T) {
	s := fixtureStore(t)
	w := workspaceFixture(t, s)
	p := profileFixture(t, s, "Work")
	s.mu.Lock()
	saved, _ := s.profileLocked(p.ID)
	delete(saved.Folders, "cursor")
	s.state.Workspaces[0].Sessions = append(s.state.Workspaces[0].Sessions, &Session{ID: "old-session", Agent: "cursor", Name: "Cursor", HasLaunched: true, ProfileID: p.ID, RepositoryID: w.Repositories[0].ID})
	changed := s.ensureProfileAccountsLocked()
	folder := saved.Folders["cursor"]
	session := s.state.Workspaces[0].Sessions[0]
	second := s.ensureProfileAccountsLocked()
	s.mu.Unlock()
	if !changed || second || folder == "" || session.ProfileID != "" {
		t.Fatalf("upgrade switched account or was not stable: %+v", session)
	}
}

func TestAmpPluginFollowsActiveThreadAndIgnoresBackgroundThreads(t *testing.T) {
	dir := t.TempDir()
	plugin := filepath.Join(dir, "plugin.mjs")
	if err := os.WriteFile(plugin, resources["AmpStatusPlugin.js"], 0600); err != nil {
		t.Fatal(err)
	}
	code := `import register from './plugin.mjs';
import {readFileSync} from 'node:fs';
const handlers={};let switched;const activeThread={current:{id:'main'},subscribe:cb=>(switched=cb,{unsubscribe(){}})};
register({on:(event,cb)=>handlers[event]=cb,activeThread,onDispose:()=>{}});
const emit=(event,id)=>handlers[event]({thread:{id}});
emit('session.start','main');emit('agent.start','main');emit('session.start','child');emit('agent.end','child');
const before=readFileSync(process.env.ORCHESTRA_STATUS_FILE,'utf8');
activeThread.current={id:'chosen'};switched(activeThread.current);emit('session.start','chosen');emit('agent.start','chosen');
emit('agent.end','main');emit('session.start','child');
if(readFileSync(process.env.ORCHESTRA_AGENT_ID_FILE,'utf8')!=='chosen'||readFileSync(process.env.ORCHESTRA_STATUS_FILE,'utf8')!=='working')throw Error('lost selected thread');
emit('agent.end','chosen');
console.log(before+' '+readFileSync(process.env.ORCHESTRA_STATUS_FILE,'utf8')+' '+readFileSync(process.env.ORCHESTRA_AGENT_ID_FILE,'utf8'));
`
	harness := filepath.Join(dir, "test.mjs")
	_ = os.WriteFile(harness, []byte(code), 0600)
	cmd := exec.Command("node", harness)
	cmd.Env = append(os.Environ(), "ORCHESTRA_AGENT_KIND=amp", "ORCHESTRA_AGENT_ID_FILE="+filepath.Join(dir, "id"), "ORCHESTRA_STATUS_FILE="+filepath.Join(dir, "status"), "ORCHESTRA_ROOT_AGENT_ID=")
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "working waiting chosen" {
		t.Fatalf("Amp status: %q %v", out, err)
	}
}

func TestLegacyHooksUseSharedRootReporter(t *testing.T) {
	for _, id := range []string{"claude", "codex", "muse"} {
		command, err := launchCommand(&Session{ID: "session", Agent: id}, t.TempDir())
		if err != nil || !strings.Contains(command, "agent-status") || strings.Contains(command, "agent-id ") {
			t.Fatalf("%s bypasses shared root filtering: %s %v", id, command, err)
		}
	}
	path := t.TempDir()
	if err := installClineHooks(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, "TaskComplete.sh"))
	if err != nil || !strings.Contains(string(data), "agent-status") {
		t.Fatal("Cline bypasses shared root filtering")
	}
}

func TestCursorDefaultConfigOverrideReceivesSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := filepath.Join(home, "Custom Cursor")
	t.Setenv("CURSOR_CONFIG_DIR", folder)
	source := filepath.Join(home, ".cursor", "skills", "orchestra", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("public Orchestra guide"), 0600); err != nil {
		t.Fatal(err)
	}
	fakeSkillShell(t, func(string) ([]byte, int, error) { return []byte("installed"), 0, nil })
	result, err := InstallAgentSkill(context.Background(), []string{"cursor"}, nil)
	if err != nil || result.ExitCode != 0 || !profileSkillInstalled(nil, "cursor") {
		t.Fatalf("Default config override did not receive its skill: %+v %v", result, err)
	}
	data, _ := os.ReadFile(filepath.Join(folder, "skills", "orchestra", "SKILL.md"))
	if string(data) != "public Orchestra guide" {
		t.Fatal("did not copy the installed public stub")
	}
}
