package core

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Let the private CLI shim exercise the real CLI in subprocesses during tests.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--orchestra-cli" {
		os.Exit(RunCLI(os.Args[2:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func TestPiAutomaticReplyReleasesWaitingSender(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node.js is required for the Pi extension integration check")
	}
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sender := sessionFixture(t, s, p, "claude")
	recipient := sessionFixture(t, s, p, "pi")
	fixture := t.TempDir()
	harness := filepath.Join(fixture, "pi.mjs")
	// Simulate Pi lifecycle events and model output; never contact a model service.
	code := `import {readFileSync} from 'node:fs';
import {join} from 'node:path';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {createInterface} from 'node:readline';
const source = readFileSync(join(process.env.ORCHESTRA_CLI_DIRECTORY,'PiStatusExtension.js'),'utf8');
const {default: register} = await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
const handlers = new Map();
register({on:(event,handler)=>handlers.set(event,handler),exec:async(command,args,options)=>{
  try {const r=await promisify(execFile)(command,args,options);return {...r,code:0};}
  catch(e){return {code:e.code||1,stdout:e.stdout||'',stderr:e.stderr||e.message};}
}});
const ctx={sessionManager:{getSessionId:()=>process.env.ORCHESTRA_SESSION_ID},ui:{notify:text=>console.error(text)}};
await handlers.get('session_start')({},ctx);
console.log('fake Pi ready');
for await (const prompt of createInterface({input:process.stdin})) {
  await handlers.get('before_agent_start')({prompt},ctx);
  await handlers.get('agent_start')({},ctx);
  await handlers.get('agent_end')({messages:[{role:'assistant',stopReason:'stop',content:[
    {type:'thinking',thinking:'Private reasoning must stay in Pi'},
    {type:'text',text:'A capital da França é Paris.'}
  ]}]},ctx);
  await handlers.get('agent_settled')({},ctx);
}
`
	for path, text := range map[string]string{
		harness:                         code,
		filepath.Join(fixture, "shell"): "#!/bin/sh\nexec /bin/sh -c \"$4\"\n",
		filepath.Join(fixture, "pi"):    "#!/bin/sh\nstty -echo\nexec " + shellQuote(node) + " " + shellQuote(harness) + "\n",
	} {
		if err := os.WriteFile(path, []byte(text), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SHELL", filepath.Join(fixture, "shell"))
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	if _, err := s.StartSession(recipient.ID, false, 100, 32); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return s.Snapshot().Projects[0].Sessions[1].Activity == "waiting" })
	state, err := s.SendRequest(sender.ID, recipient.ID, "Qual é a capital da França?")
	if err != nil {
		t.Fatal(err)
	}
	runID := state.Runs[0].ID
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(sender.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	waiter := exec.Command(filepath.Join(s.runtimeDir, "orchestra"), "run", "wait", runID, "--timeout", "5s")
	waiter.Env = environmentList(environmentMap())
	for key, value := range env {
		waiter.Env = append(waiter.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	waiter.Stdout, waiter.Stderr = &stdout, &stderr
	if err := waiter.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = waiter.Process.Kill() })
	done := make(chan error, 1)
	go func() { done <- waiter.Wait() }()
	waitUntil(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, waiting := s.collaboration.waits[sender.ID]
		return waiting
	})
	if _, err := s.ExecuteRequest(runID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sender did not receive reply: %v: %s", err, stderr.String())
		}
	case <-time.After(6 * time.Second):
		t.Fatal("sender remained blocked")
	}
	var received Run
	if err := json.Unmarshal(stdout.Bytes(), &received); err != nil {
		t.Fatal(err)
	}
	if received.ID != runID || received.Status != "completed" || received.Response != "A capital da França é Paris." {
		t.Fatalf("wrong response delivered: %+v", received)
	}
}
