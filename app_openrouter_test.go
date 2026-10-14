package main

import "testing"

func TestOpenRouterKeyServiceIsPerProfile(t *testing.T) {
	t.Setenv("ORCHESTRA_STATE_FILE", "")
	if got := openRouterKeyService(""); got != "com.orchestra.openrouter" {
		t.Fatalf("Default service = %q", got)
	}
	if got := openRouterKeyService("4b8f90f1-3922-4ddb-99a2-84c9daf067d6"); got != "com.orchestra.openrouter.profile-4b8f90f1-3922-4ddb-99a2-84c9daf067d6" {
		t.Fatalf("profile service = %q", got)
	}
	t.Setenv("ORCHESTRA_STATE_FILE", "/tmp/qa/state.json")
	if a, b := openRouterKeyService(""), openRouterKeyService("p"); a == "com.orchestra.openrouter" || b != a+".profile-p" {
		t.Fatalf("QA services = %q, %q", a, b)
	}
}
