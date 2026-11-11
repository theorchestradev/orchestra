package core

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// cliGuide is the agent-facing guide printed by orchestra guide. The installable skill
// stub in skills/orchestra only points here, so the guide always matches this binary.
//
//go:embed resources/guide.md
var cliGuide string

// Version is the app version that orchestra --version prints. The app sets it from
// main.version, which release builds take from the v* tag.
var Version = "dev"

// cliOptions lists each command's options, so a misplaced one fails instead of being ignored.
var cliOptions = map[string][]string{
	"automation.list":   {"--workspace"},
	"automation.show":   nil,
	"automation.create": {"--name", "--agent", "--model", "--effort", "--prompt", "--prompt-file", "--stdin", "--repository", "--time", "--date", "--timezone", "--days", "--enabled", "--disabled", "--workspace"},
	"automation.update": {"--name", "--agent", "--model", "--effort", "--prompt", "--prompt-file", "--stdin", "--repository", "--time", "--date", "--timezone", "--days", "--enabled", "--disabled"},
	"automation.enable": nil, "automation.disable": nil, "automation.run": nil, "automation.delete": nil,
	"session.create":    {"--agent", "--model", "--effort", "--prompt", "--prompt-file", "--stdin", "--name", "--workspace", "--repository", "--worktree", "--branch", "--base", "--request-id", "--start"},
	"session.list":      nil,
	"session.rename":    {"--name", "--workspace"},
	"session.close":     {"--workspace"},
	"session.inbox":     nil,
	"session.request":   {"--text", "--file", "--request-id"},
	"session.read":      {"--before"},
	"run.read":          nil,
	"run.wait":          {"--timeout"},
	"run.reply":         {"--text", "--file"},
	"run.cancel":        nil,
	"scratchpad.read":   {"--json"},
	"scratchpad.write":  {"--text", "--file", "--stdin", "--if-revision"},
	"scratchpad.append": {"--text", "--file", "--stdin"},
}

const cliHelp = `Orchestra CLI

orchestra guide
orchestra --version
orchestra automation list [--workspace ID_OR_NAME]
orchestra automation show AUTOMATION_ID
orchestra automation create --name NAME --agent AGENT (--prompt TEXT | --prompt-file PATH | --stdin) --time HH:MM (--days daily|mon,wed,... | --date YYYY-MM-DD) [--timezone IANA_ZONE] [--workspace ID_OR_NAME] [--repository ID] [--model MODEL] [--effort EFFORT] [--enabled]
orchestra automation update AUTOMATION_ID [--name NAME] [--agent AGENT] [--model MODEL] [--effort EFFORT] [--prompt TEXT | --prompt-file PATH | --stdin] [--repository ID] [--time HH:MM] [--days daily|once|mon,wed,...] [--date YYYY-MM-DD] [--timezone IANA_ZONE] [--enabled | --disabled]
orchestra automation enable AUTOMATION_ID
orchestra automation disable AUTOMATION_ID
orchestra automation run AUTOMATION_ID
orchestra automation delete AUTOMATION_ID
orchestra session list
orchestra session create --agent AGENT [--model MODEL] [--effort EFFORT] [--prompt TEXT | --prompt-file PATH | --stdin] [--name NAME] [--workspace ID_OR_NAME] [--repository ID] [--worktree [--branch NAME] [--base REF]] [--request-id UUID] [--start]
orchestra session rename SESSION_ID --name NAME [--workspace ID_OR_NAME]
orchestra session close SESSION_ID [--workspace ID_OR_NAME]
orchestra session inbox
orchestra session read SESSION_ID [--before REQUEST_ID]
orchestra session request SESSION_ID --text TEXT [--request-id UUID]
orchestra run read REQUEST_ID
orchestra run wait REQUEST_ID [--timeout 120s]
orchestra run reply REQUEST_ID (--text TEXT | --file PATH)
orchestra run cancel REQUEST_ID
orchestra scratchpad read [--json]
orchestra scratchpad write (--text TEXT | --file PATH | --stdin) [--if-revision REV]
orchestra scratchpad append (--text TEXT | --file PATH | --stdin)

Automations are global. list returns all automations unless --workspace filters it.
Stable IDs target automations globally. create defaults to the calling session's workspace
and repository; --workspace selects by ID or exact name, and --repository must belong to it.
A different workspace uses its default repository. update preserves omitted fields and cannot
move workspaces. --repository changes the repository inside the saved workspace.
Creation is disabled unless --enabled is supplied. Timezone defaults to UTC; --timezone
accepts an IANA zone such as America/Sao_Paulo. --days accepts daily, once or comma-separated
sun,mon,tue,wed,thu,fri,sat. once requires --date. All triggers require --time.
run uses the existing app execution path without changing enabled state or next run.
Agents must create, activate or run automations only when the user requests it.
Automation commands return JSON and require the authenticated session connection.

The Scratchpad belongs to the session's workspace. read --json includes the revision
that write --if-revision expects. append adds the text on a new line.

orchestra guide prints a guide for agents and works anywhere. Other commands
run from an Orchestra terminal session while the app is open.
session create defaults to the calling session's workspace. --workspace selects a workspace
by ID or exact name; ambiguous names need an ID. --repository must belong to that workspace.
session rename and session close also accept --workspace; other commands stay in the calling
workspace. session close stops the process and removes the session, keeping its worktree.
session create starts the agent only with --start, which needs Let Agents Start Sessions turned on in Settings. --worktree gives
the session its own branch and folder, and a repeated --request-id returns the same session.
Receiving messages never starts an agent or injects terminal input.
`

func CallBroker(path string, request WireRequest) (json.RawMessage, error) {
	deadline := 10 * time.Second
	if request.Method == "session.create" {
		deadline = sessionCreateTimeout
	}
	conn, err := net.DialTimeout("unix", path, 10*time.Second)
	if err != nil {
		return nil, errors.New("Could not connect to Orchestra. Keep the app open and start this CLI from an Orchestra session.")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(deadline))
	if err = json.NewEncoder(conn).Encode(request); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(io.LimitReader(conn, 2*1024*1024+1))
	data, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(data) > 2*1024*1024 {
		return nil, errors.New("Response exceeded the size limit.")
	}
	var failure struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &failure)
	if failure.Error != "" {
		return nil, errors.New(failure.Error)
	}
	return json.RawMessage(strings.TrimSpace(string(data))), nil
}
func RunCLI(args []string, out, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[len(args)-1] == "--help" {
		fmt.Fprint(out, cliHelp)
		return 0
	}
	if args[0] == "guide" {
		fmt.Fprint(out, cliGuide)
		return 0
	}
	if args[0] == "--version" {
		fmt.Fprintln(out, "Orchestra "+Version)
		return 0
	}
	var err error
	if args[0] == "agent-id" {
		err = saveAgentID(os.Stdin, os.Getenv("ORCHESTRA_AGENT_ID_FILE"))
	} else if args[0] == "agent-status" && (len(args) == 2 || len(args) == 3) {
		err = reportAgentStatus(os.Stdin, args[1], os.Getenv("ORCHESTRA_STATUS_FILE"), os.Getenv("ORCHESTRA_AGENT_ID_FILE"), args[2:]...)
	} else {
		err = runCLI(args, out)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// saveAgentID stores the session_id from an agent hook payload, so hooks need no
// platform-specific JSON tools. Kept for hooks from earlier app versions and not listed in help.
func saveAgentID(in io.Reader, path string) error {
	defer io.Copy(io.Discard, in)
	var payload struct {
		SessionID string `json:"session_id"`
		// Cline names its conversation in sessionContext.rootSessionId.
		SessionContext struct {
			RootSessionID string `json:"rootSessionId"`
		} `json:"sessionContext"`
	}
	if err := json.NewDecoder(io.LimitReader(in, 16*1024*1024)).Decode(&payload); err != nil {
		return err
	}
	if payload.SessionID == "" {
		payload.SessionID = payload.SessionContext.RootSessionID
	}
	if path == "" || payload.SessionID == "" {
		return errors.New("Missing agent session ID.")
	}
	return atomicWrite(path, []byte(payload.SessionID), 0600)
}
func runCLI(args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("Expected session, run, automation or scratchpad command. Use orchestra help.")
	}
	method := args[0] + "." + args[1]
	options, valid := cliOptions[method]
	if !valid {
		return errors.New("Unknown command. Use orchestra help.")
	}
	env := environmentMap()
	if path := env["ORCHESTRA_CONTEXT"]; path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var context map[string]string
		if err = json.Unmarshal(data, &context); err != nil {
			return err
		}
		for k, v := range context {
			if env[k] == "" {
				env[k] = v
			}
		}
	}
	request := WireRequest{Method: method, Source: env["ORCHESTRA_SESSION_ID"], Token: env["ORCHESTRA_TOKEN"]}
	path := env["ORCHESTRA_SOCKET"]
	if path == "" || request.Source == "" || request.Token == "" {
		return errors.New("Could not connect to Orchestra. Keep the app open and start this CLI from an Orchestra session.")
	}
	index := 2
	if method != "session.list" && method != "session.inbox" && method != "session.create" && method != "automation.create" && method != "automation.list" && args[0] != "scratchpad" {
		if len(args) <= index || strings.HasPrefix(args[index], "--") {
			return errors.New("A session, request or automation ID is required.")
		}
		request.Target = args[index]
		index++
	}
	timeout := 120 * time.Second
	textSet, printJSON := false, false
	onlyOneText := errors.New("Use only one of --text, --file or --stdin.")
	if method == "session.create" || strings.HasPrefix(method, "automation.") {
		onlyOneText = errors.New("Use only one of --prompt, --prompt-file or --stdin.")
	}
	if strings.HasPrefix(method, "automation.") {
		request.Automation = &AutomationPatch{}
	}
	activationSet := false
	for index < len(args) {
		flag := args[index]
		index++
		if !slices.Contains(options, flag) {
			return fmt.Errorf("Unknown option %s.", flag)
		}
		if flag == "--enabled" || flag == "--disabled" {
			if activationSet {
				return errors.New("Use only one of --enabled or --disabled.")
			}
			activationSet = true
			enabled := flag == "--enabled"
			request.Automation.Enabled = &enabled
			continue
		}
		if flag == "--json" {
			printJSON = true
			continue
		}
		if flag == "--start" {
			request.Start = true
			continue
		}
		if flag == "--worktree" {
			request.Worktree = true
			continue
		}
		if flag == "--stdin" {
			if textSet {
				return onlyOneText
			}
			textSet = true
			data, err := readLimited(os.Stdin)
			if err != nil {
				return err
			}
			request.Text = data
			continue
		}
		if index >= len(args) {
			return fmt.Errorf("Missing value for %s.", flag)
		}
		value := args[index]
		index++
		if request.Automation != nil {
			patch := request.Automation
			fields := map[string]**string{"--name": &patch.Name, "--agent": &patch.Agent, "--model": &patch.Model, "--effort": &patch.Effort, "--repository": &patch.RepositoryID, "--time": &patch.Time, "--date": &patch.Date, "--timezone": &patch.TimeZone}
			if field := fields[flag]; field != nil {
				*field = &value
				continue
			}
			if flag == "--days" {
				days, err := automationDays(value)
				if err != nil {
					return err
				}
				patch.Days = &days
				continue
			}
		}
		switch flag {
		case "--agent":
			request.Agent = value
		case "--model":
			request.Model = value
		case "--effort":
			request.Effort = value
		case "--name":
			request.Name = value
		case "--workspace":
			if value == "" {
				return errors.New("The workspace must be an ID or exact name.")
			}
			request.Workspace = value
		case "--repository":
			request.RepositoryID = value
		case "--branch":
			request.Branch = value
		case "--base":
			request.Base = value
		case "--text", "--file", "--prompt", "--prompt-file":
			if textSet {
				return onlyOneText
			}
			textSet = true
			request.Text = value
			if flag == "--file" || flag == "--prompt-file" {
				f, err := os.Open(value)
				if err != nil {
					return err
				}
				request.Text, err = readLimited(f)
				_ = f.Close()
				if err != nil {
					return err
				}
			}
		case "--if-revision":
			revision, err := strconv.Atoi(value)
			if err != nil || revision < 0 {
				return errors.New("The revision must be a non-negative number.")
			}
			request.Revision = &revision
		case "--request-id":
			request.RequestID = value
		case "--before":
			request.Before = value
		case "--timeout":
			duration, err := time.ParseDuration(value)
			if err != nil {
				seconds, e := strconv.Atoi(value)
				if e != nil {
					return err
				}
				duration = time.Duration(seconds) * time.Second
			}
			if duration < 0 || duration > 24*time.Hour {
				return errors.New("Timeout must be between 0 and 24 hours.")
			}
			timeout = duration
		}
	}
	if request.Automation != nil && textSet {
		request.Automation.Prompt = &request.Text
	}
	if method == "session.rename" && strings.TrimSpace(request.Name) == "" {
		return errors.New("Use --name with a non-empty session name.")
	}
	if method == "session.create" && request.Agent == "" {
		return errors.New("Use --agent to create a session.")
	}
	if !request.Worktree && (request.Branch != "" || request.Base != "") {
		return errors.New("--branch and --base need --worktree.")
	}
	if method == "session.create" && textSet && strings.TrimSpace(request.Text) == "" {
		return errors.New("The prompt is empty. Omit it to create a session without one.")
	}
	if (method == "session.request" || method == "run.reply") && !textSet {
		return errors.New("Use --text or --file to supply a message.")
	}
	if (method == "scratchpad.write" || method == "scratchpad.append") && !textSet {
		return errors.New("Use --text, --file or --stdin to supply the text.")
	}
	if method == "scratchpad.read" && !printJSON {
		data, err := CallBroker(path, request)
		if err != nil {
			return err
		}
		var pad struct {
			Text string `json:"text"`
		}
		if err = json.Unmarshal(data, &pad); err != nil {
			return err
		}
		fmt.Fprint(out, pad.Text)
		if pad.Text != "" && !strings.HasSuffix(pad.Text, "\n") {
			fmt.Fprintln(out)
		}
		return nil
	}
	if method != "run.wait" {
		data, err := CallBroker(path, request)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
	}
	defer func() { clear := request; clear.Method = "run.wait.clear"; _, _ = CallBroker(path, clear) }()
	deadline := time.Now().Add(timeout)
	for {
		data, err := CallBroker(path, request)
		if err != nil {
			return err
		}
		var run Run
		if err = json.Unmarshal(data, &run); err != nil {
			return err
		}
		if run.finished() {
			fmt.Fprintln(out, string(data))
			return nil
		}
		if !time.Now().Before(deadline) {
			return errors.New("Wait timed out. The request was not cancelled.")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// readLimited reads message text from a file or standard input up to the 256 KB limit.
func readLimited(in io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(in, 256*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 256*1024 {
		return "", errors.New("Input exceeds 256 KB.")
	}
	return string(data), nil
}
