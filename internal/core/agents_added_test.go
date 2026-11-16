package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Exercise the actual shell boundary against fake CLIs, including quote/control
// characters. These scripts only print arguments; they cannot contact providers.
func TestAgentInteractiveLaunches(t *testing.T) {
	const conversation = "e3d31e08-8a38-4456-845d-f00f692cc001"
	for _, spec := range agentOrder {
		id := spec.ID
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			script := "#!/bin/sh\nif [ \"$1\" = create-chat ]; then printf '%s' " + conversation + "; exit; fi\nprintf '%s\\0' \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, spec.Executable), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			sess := &Session{ID: conversation, Agent: id}
			if spec.Model != nil {
				sess.Model = "provider/model"
			}
			term := &TerminalSession{idFile: filepath.Join(dir, "id")}
			store := &Store{path: filepath.Join(dir, "state.json"), runtimeDir: dir}
			env := map[string]string{"PATH": dir + ":" + os.Getenv("PATH"), "ORCHESTRA_AGENT_ID_FILE": term.idFile}
			prompt := "--print quotes ' \" $(touch must-not-exist)\n/auto"
			for _, resumed := range []bool{false, true} {
				sess.HasLaunched = resumed
				sess.AgentSessionID = conversation
				if spec.Prepare != nil {
					if err := spec.Prepare(store, sess, term, env); err != nil {
						t.Fatal(err)
					}
				}
				sess.InitialPrompt = ""
				if !resumed && !spec.NoInitialPrompt {
					sess.InitialPrompt = prompt
				}
				command, err := launchCommand(sess, dir)
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("/bin/sh", "-c", "exec "+command)
				cmd.Dir = dir
				cmd.Env = environmentList(env)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v: %s", command, err, out)
				}
				args := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
				if spec.Model != nil && spec.Model.Flag != "" && !slices.Contains(args, sess.Model) {
					t.Fatalf("lost model choice in %q", args)
				}
				if id == "goose" && env["GOOSE_MODEL"] != sess.Model {
					t.Fatal("Goose did not receive the selected model")
				}
				if id == "droid" {
					data, _ := os.ReadFile(filepath.Join(dir, sess.ID+"-droid-settings.json"))
					if !strings.Contains(string(data), sess.Model) {
						t.Fatal("Droid did not receive the selected model")
					}
				}
				if !resumed && !spec.NoInitialPrompt && !slices.Contains(args, prompt) {
					t.Fatalf("lost prompt in %q", args)
				}
				if _, err := os.Stat(filepath.Join(dir, "must-not-exist")); !os.IsNotExist(err) {
					t.Fatal("prompt executed as shell code")
				}
				if resumed && id != "aider" && id != "copilot" && !slices.Contains(args, conversation) {
					t.Fatalf("lost conversation in %q", args)
				}
				if id == "aider" && resumed && !slices.Contains(args, "--restore-chat-history") {
					t.Fatal("Aider did not restore history")
				}
				if id == "copilot" || id == "qwen" {
					saved, _ := os.ReadFile(term.idFile)
					if env["ORCHESTRA_CONVERSATION_ID"] != string(saved) || !slices.Contains(args, string(saved)) {
						t.Fatalf("conversation capture disagrees with %q", args)
					}
					if resumed && string(saved) != conversation {
						t.Fatal("resumed another conversation")
					}
				}
				for _, forbidden := range []string{"--print", "--yolo", "--auto", "--allow-all", "--trust-all-tools", "--skip-permissions-unsafe"} {
					if slices.Contains(args, forbidden) {
						t.Fatalf("non-interactive/bypass argument %q", forbidden)
					}
				}
			}
		})
	}
}

func TestInteractiveOnlyAgentsRejectInitialPrompts(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	for _, id := range []string{"aider", "kimi", "amp"} {
		if _, err := s.CreateSession(p.ID, "", id, "", "request", true, nil, "", "", "", "", ""); err == nil || !strings.Contains(err.Error(), "initial prompt") {
			t.Fatalf("%s: %v", id, err)
		}
		if _, err := launchCommand(&Session{ID: "id", Agent: id, InitialPrompt: "request"}, t.TempDir()); err == nil {
			t.Fatalf("%s silently dropped a saved prompt", id)
		}
	}
	if len(s.Snapshot().Workspaces[0].Sessions) != 0 {
		t.Fatal("rejected prompt created a session")
	}
}

func TestAiderConversationHistoryIsolation(t *testing.T) {
	dir := t.TempDir()
	s := &Store{path: filepath.Join(dir, "state.json")}
	sess := &Session{ID: "session", Agent: "aider"}
	term := &TerminalSession{}
	env := map[string]string{}
	if err := aiderAgent.Prepare(s, sess, term, env); err != nil {
		t.Fatal(err)
	}
	first := term.agentDirectory
	id := aiderAgent.CaptureID(term)
	if err := os.WriteFile(filepath.Join(first, "chat.md"), []byte("keep history"), 0600); err != nil {
		t.Fatal(err)
	}
	sess.HasLaunched, sess.AgentSessionID = true, id
	if err := aiderAgent.Prepare(s, sess, term, env); err != nil || term.agentDirectory != first {
		t.Fatalf("resume lost history: %v", err)
	}
	sess.HasLaunched = false
	if err := aiderAgent.Prepare(s, sess, term, env); err != nil || term.agentDirectory == first {
		t.Fatalf("new conversation reused history: %v", err)
	}
	sess.HasLaunched, sess.AgentSessionID = true, "../../outside"
	if err := aiderAgent.Prepare(s, sess, term, env); err == nil {
		t.Fatal("accepted a path as a conversation ID")
	}
}

func TestAddedResumeCommandsDoNotCaptureFlags(t *testing.T) {
	for _, id := range []string{"kiro", "droid"} {
		for _, text := range []string{"droid resume --last", "droid --resume --last", "kiro-cli chat --resume-id --help"} {
			if agentSpecs[id].RecoverID.MatchString(text) {
				t.Fatalf("%s captured a flag in %q", id, text)
			}
		}
	}
}

func TestCursorChatCreationFailureDoesNotResumeLatest(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = create-chat ]; then exit 1; fi\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = orchestra-chat-creation-failed ]; then exit 7; fi; shift; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "cursor-agent"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	idFile := filepath.Join(dir, "id")
	command, err := launchCommand(&Session{ID: "id", Agent: "cursor"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", "exec "+command)
	cmd.Env = []string{"PATH=" + dir + ":" + os.Getenv("PATH"), "ORCHESTRA_AGENT_ID_FILE=" + idFile}
	if err := cmd.Run(); err == nil {
		t.Fatal("failed creation fell back to an empty resume ID")
	}
	if _, err := os.Stat(idFile); !os.IsNotExist(err) {
		t.Fatal("failed creation captured an ID")
	}
}

func TestPresetAgentsStartNewConversationWithFreshID(t *testing.T) {
	for _, id := range []string{"claude", "grok", "pi"} {
		t.Run(id, func(t *testing.T) {
			fakeAgent(t, "printf '%s\\n' \"$@\"\n")
			bin := filepath.SplitList(os.Getenv("PATH"))[0]
			if id != "claude" {
				if err := os.Symlink(filepath.Join(bin, "claude"), filepath.Join(bin, agentSpecs[id].Executable)); err != nil {
					t.Fatal(err)
				}
			}
			s := fixtureStore(t)
			workspace := workspaceFixture(t, s)
			session := sessionFixture(t, s, workspace, id)
			previous := session.ID
			for range 2 {
				if _, err := s.StartSession(session.ID, true, 80, 24); err != nil {
					t.Fatal(err)
				}
				waitUntil(t, func() bool { return !sessionState(s, session.ID).Running })
				current := sessionState(s, session.ID).AgentSessionID
				if current == "" || current == previous || !strings.Contains(terminalText(s, session.ID), current) {
					t.Fatalf("new conversation reused an ID or launched another one: previous=%s current=%s", previous, current)
				}
				previous = current
			}
		})
	}
}

func TestAgentPreparationSurvivesShellStartupOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// The app may inherit exactly the selected values; this is still an explicit
	// launch choice that rc files must not replace.
	t.Setenv("GOOSE_MODEL", "chosen/model")
	t.Setenv("GOOSE_MODE", "smart_approve")
	fakeAgent(t, "printf 'model=%s mode=%s\\r\\n' \"$GOOSE_MODEL\" \"$GOOSE_MODE\"\n")
	bin := filepath.SplitList(os.Getenv("PATH"))[0]
	if err := os.Symlink(filepath.Join(bin, "claude"), filepath.Join(bin, "goose")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SHELL"), []byte("#!/bin/sh\nexport GOOSE_MODEL=wrong-model GOOSE_MODE=auto\nexec /bin/sh -c \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s := fixtureStore(t)
	w := workspaceFixture(t, s)
	state, err := s.CreateSession(w.ID, "", "goose", "", "", false, nil, "chosen/model", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session := state.Workspaces[0].Sessions[0]
	if _, err := s.StartSession(session.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return !sessionState(s, session.ID).Running })
	if !strings.Contains(terminalText(s, session.ID), "model=chosen/model mode=smart_approve") {
		t.Fatalf("shell startup replaced launch settings: %q", terminalText(s, session.ID))
	}
}
