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

const cliHelp = `Orchestra session messaging

orchestra guide
orchestra --version
orchestra session list
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

The Scratchpad belongs to the session's workspace. read --json includes the revision
that write --if-revision expects. append adds the text on a new line.

orchestra guide prints a guide for agents and works anywhere. Other commands
run from an Orchestra terminal session while the app is open.
Receiving messages never starts an agent or injects terminal input.
`

func CallBroker(path string, request WireRequest) (json.RawMessage, error) {
	conn, err := net.DialTimeout("unix", path, 10*time.Second)
	if err != nil {
		return nil, errors.New("Could not connect to Orchestra. Keep the app open and start this CLI from an Orchestra session.")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
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
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
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
// platform-specific JSON tools. It is used by statusCommand and not listed in help.
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
		return errors.New("Expected session, run or scratchpad command. Use orchestra help.")
	}
	method := args[0] + "." + args[1]
	valid := map[string]bool{"session.list": true, "session.inbox": true, "session.request": true, "session.read": true, "run.read": true, "run.wait": true, "run.reply": true, "run.cancel": true, "scratchpad.read": true, "scratchpad.write": true, "scratchpad.append": true}
	if !valid[method] {
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
	scratchpad := args[0] == "scratchpad"
	if method != "session.list" && method != "session.inbox" && !scratchpad {
		if len(args) <= index || strings.HasPrefix(args[index], "--") {
			return errors.New("A session or request ID is required.")
		}
		request.Target = args[index]
		index++
	}
	timeout := 120 * time.Second
	textSet, printJSON := false, false
	for index < len(args) {
		flag := args[index]
		index++
		if flag == "--json" && method == "scratchpad.read" {
			printJSON = true
			continue
		}
		if flag == "--stdin" && scratchpad && method != "scratchpad.read" {
			if textSet {
				return errors.New("Use only one of --text, --file or --stdin.")
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
		switch flag {
		case "--text", "--file":
			if textSet {
				return errors.New("Use only one of --text, --file or --stdin.")
			}
			textSet = true
			request.Text = value
			if flag == "--file" {
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
			if method != "scratchpad.write" {
				return fmt.Errorf("Unknown option %s.", flag)
			}
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
		default:
			return fmt.Errorf("Unknown option %s.", flag)
		}
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
