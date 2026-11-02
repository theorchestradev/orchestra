package core

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// agentSpec is everything Orchestra knows about one agent CLI. Each agent lives in
// its own agent_<id>.go file; the rest of the package asks a spec what an agent
// supports instead of checking agent IDs. Sessions stay owned by TerminalSession: a
// spec only describes how to start and observe its CLI.
//
// Agents start in their auto mode: routine approvals are skipped while a safety layer
// still reviews risky actions. Full bypass flags are never used. Vibe, OpenCode and Kilo
// only offer approve-all modes, so they keep their default prompts; Pi has no tool approval
// prompts. Antigravity and omp have no reviewed auto mode, so they only auto-approve
// edits. Cline approves every tool by default, so it starts with approvals turned back on.
// Muse's built-in :auto-review permission profile is its reviewed auto mode.
// Qwen uses its auto classifier and Goose uses smart_approve. Other adapters keep
// their CLI's interactive permission defaults; none enable full bypass.
type agentSpec struct {
	ID, Name string
	// Executable is the CLI on the login-shell PATH, for detection and launch errors.
	Executable string
	// SkillKey is the agent key of the pinned skills CLI; empty means never installed to.
	SkillKey string
	// Command builds the shell command that starts or resumes the agent.
	Command func(l launch) string
	// NoInitialPrompt means the CLI has no reviewed way to seed its interactive UI.
	NoInitialPrompt bool
	// Prepare adjusts the launch environment, before Command runs in it.
	Prepare func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error
	// OverrideEnv are prepared values that must override shell rc files even when
	// they already matched the app's inherited environment before Prepare.
	OverrideEnv []string
	// PresetID agents take Orchestra's session ID as their conversation ID.
	PresetID bool
	// RecoverID finds the conversation ID in the terminal output once the agent exits.
	RecoverID *regexp.Regexp
	// CaptureID reads the current conversation ID while the agent runs; empty means none yet.
	// The generic ORCHESTRA_AGENT_ID_FILE needs no CaptureID.
	CaptureID func(t *TerminalSession) string
	// ResumeFailed matches the output of a resume that exited because the agent has no
	// conversation with the saved ID, so the session offers a new conversation instead.
	ResumeFailed *regexp.Regexp
	// StatusSetup agents report status only after the user enables their hooks, so a
	// silent status file shows a setup hint.
	StatusSetup bool
	// ForwardsReplies agents answer Inbox requests through their own extension.
	ForwardsReplies bool
	// Nil capabilities are unsupported. A model capability without a catalog
	// accepts a manually entered ID; without flags it is applied by Prepare.
	Model   *agentModelSpec
	Account *agentAccountSpec
}

// agentModelSpec describes choices independently of a provider's transport.
// Prepare applies choices that the CLI receives through settings or environment.
type agentModelSpec struct {
	Flag string
	// Env names the model transport when Prepare uses an environment variable.
	Env        string
	Command    []string
	Parse      func([]byte) ([]AgentModel, error)
	Catalog    []AgentModel
	EffortFlag func(string) string
	// Nil with EffortFlag accepts provider-defined values; otherwise this is the allowlist.
	Efforts []string
}

// agentAccountSpec describes account isolation, sign-in and skill locations.
// Status is a read-only CLI command. ParseStatus handles CLIs that exit zero
// even when signed out; Orchestra never reads authentication files.
type agentAccountSpec struct {
	Env           string
	Aliases       []string
	Login, Status string
	ParseStatus   func([]byte) bool
	SkillDir      string
	// Some skills installers ignore Env. Copy their installed public stub instead.
	CopySkill bool
}

// launch is what an agent's Command receives. Resume is the conversation to continue,
// Flags the model and effort flags and Prompt the quoted initial prompt.
type launch struct {
	Session                    *Session
	Resume, Flags, Prompt, Dir string
}

// conversationID is preset for providers that accept a client-chosen ID. Starting
// a new conversation changes this ID without changing Orchestra's session ID.
func (l launch) conversationID() string {
	if l.Session.AgentSessionID != "" {
		return l.Session.AgentSessionID
	}
	return l.Session.ID
}

// agentOrder is the app's agent order; frontend/src/shared/appearance.js lists the same agents.
var agentOrder = []*agentSpec{codexAgent, claudeAgent, piAgent, vibeAgent, opencodeAgent, antigravityAgent, grokAgent, clineAgent, ompAgent, kiloAgent, museAgent, cursorAgent, copilotAgent, aiderAgent, gooseAgent, qwenAgent, kiroAgent, droidAgent, kimiAgent, ampAgent}

var agentSpecs = func() map[string]*agentSpec {
	m := map[string]*agentSpec{}
	for _, spec := range agentOrder {
		m[spec.ID] = spec
	}
	return m
}()

// specIndex maps the agents whose field is set to its value.
func specIndex[T comparable](field func(*agentSpec) T) map[string]T {
	m, zero := map[string]T{}, *new(T)
	for _, spec := range agentOrder {
		if value := field(spec); value != zero {
			m[spec.ID] = value
		}
	}
	return m
}

var (
	agentConfigEnv = specIndex(func(s *agentSpec) string {
		if s.Account != nil {
			return s.Account.Env
		}
		return ""
	})
	loginCommands = specIndex(func(s *agentSpec) string {
		if s.Account != nil {
			return s.Account.Login
		}
		return ""
	})
	loginStatusCommands = specIndex(func(s *agentSpec) string {
		if s.Account != nil {
			return s.Account.Status
		}
		return ""
	})
	profileSkillDirs = specIndex(func(s *agentSpec) string {
		if s.Account != nil {
			return s.Account.SkillDir
		}
		return ""
	})
)

// presetsID reports whether new sessions of the agent start with Orchestra's ID.
func presetsID(agent string) bool {
	spec, ok := agentSpecs[agent]
	return ok && spec.PresetID
}

// hookEvents are the status hook events agents with hooks share, and hookState is the
// status each one reports.
var hookEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Stop"}

func hookState(event string) string {
	if event == "UserPromptSubmit" || event == "PreToolUse" {
		return "working"
	}
	return "waiting"
}

// command appends the resume flag, model and effort flags and prompt flag to base.
func (l launch) command(base, resumeFlag, promptFlag string) string {
	command := base
	if l.Resume != "" {
		command += " " + resumeFlag + " " + shellQuote(l.Resume)
	}
	command += l.Flags
	if l.Prompt != "" {
		command += " " + promptFlag + " " + l.Prompt
	}
	return command
}

// quotedFlag renders " flag 'value'" for EffortFlag.
func quotedFlag(flag string) func(string) string {
	return func(value string) string { return fmt.Sprintf(" %s %s", flag, shellQuote(value)) }
}

// modelCatalog collects listed models, skipping invalid and repeated IDs.
type modelCatalog []AgentModel

func (c *modelCatalog) add(id, name string, efforts []string) {
	if modelPattern.MatchString(id) && !slices.ContainsFunc(*c, func(m AgentModel) bool { return m.ID == id }) {
		*c = append(*c, AgentModel{id, name, efforts})
	}
}

// scanModels runs line once per output line.
func scanModels(output []byte, line func(c *modelCatalog, text string, fields []string)) ([]AgentModel, error) {
	c := modelCatalog{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line(&c, scanner.Text(), strings.Fields(scanner.Text()))
	}
	return c, scanner.Err()
}

// prepareConversationID scopes CLI conversations to an Orchestra session without
// reusing another conversation when Start New Conversation is chosen.
func prepareConversationID(_ *Store, sess *Session, t *TerminalSession, env map[string]string) error {
	id := sess.AgentSessionID
	if !sess.HasLaunched || id == "" {
		id = uuid.NewString()
	}
	env["ORCHESTRA_CONVERSATION_ID"] = id
	return os.WriteFile(t.idFile, []byte(id), 0600)
}
