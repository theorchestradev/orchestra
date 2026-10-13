package core

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// agentSpec is everything Orchestra knows about one agent CLI. Each agent lives in
// its own agent_<id>.go file; the rest of the package asks a spec what an agent
// supports instead of checking agent IDs. Sessions stay owned by TerminalSession: a
// spec only describes how to start and observe its CLI.
//
// Agents start in their auto mode: routine approvals are skipped while a safety layer
// still reviews risky actions. Full bypass flags are never used. Vibe and OpenCode only
// offer approve-all modes, so they keep their default prompts; Pi has no tool approval
// prompts. Antigravity has no reviewed auto mode, so it only auto-approves edits.
type agentSpec struct {
	ID, Name string
	// Executable is the CLI on the login-shell PATH, for detection and launch errors.
	Executable string
	// SkillKey is the agent key of the pinned skills CLI; empty means never installed to.
	SkillKey string
	// Command builds the shell command that starts or resumes the agent.
	Command func(l launch) string
	// Prepare adjusts the launch environment, before Command runs in it.
	Prepare func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error
	// PresetID agents take Orchestra's session ID as their conversation ID.
	PresetID bool
	// RecoverID finds the conversation ID in the terminal output once the agent exits.
	RecoverID *regexp.Regexp
	// CaptureID reads the current conversation ID while the agent runs; empty means none yet.
	// The generic ORCHESTRA_AGENT_ID_FILE needs no CaptureID.
	CaptureID func(t *TerminalSession) string
	// StatusSetup agents report status only after the user enables their hooks, so a
	// silent status file shows a setup hint.
	StatusSetup bool
	// ForwardsReplies agents answer Inbox requests through their own extension.
	ForwardsReplies bool
	// ModelFlag passes a model at launch; empty means the agent takes no model choice.
	ModelFlag string
	// ModelCommand lists the installed CLI's models for ParseModels. Without one,
	// Models is the fixed catalog.
	ModelCommand []string
	ParseModels  func(output []byte) ([]AgentModel, error)
	Models       []AgentModel
	// EffortFlag passes an effort at launch; nil means the agent takes no effort choice.
	// Efforts lists the accepted values; nil accepts any effortPattern value.
	EffortFlag func(effort string) string
	Efforts    []string
}

// launch is what an agent's Command receives. Resume is the conversation to continue,
// Flags the model and effort flags and Prompt the quoted initial prompt.
type launch struct {
	Session                    *Session
	Resume, Flags, Prompt, Dir string
}

// agentOrder is the app's agent order; frontend/src/shared/appearance.js lists the same agents.
var agentOrder = []*agentSpec{codexAgent, claudeAgent, piAgent, vibeAgent, opencodeAgent, antigravityAgent, grokAgent}

var agentSpecs = func() map[string]*agentSpec {
	m := map[string]*agentSpec{}
	for _, spec := range agentOrder {
		m[spec.ID] = spec
	}
	return m
}()

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
