package main

import "testing"

func TestLinearCredentialScope(t *testing.T) {
	t.Setenv("ORCHESTRA_STATE_FILE", "")
	first := linearKeyService("first")
	if first == linearKeyService("second") || first == "com.orchestra.linear" {
		t.Fatal("workspace credentials must be separate from each other and the legacy global key")
	}
	if first != linearKeyService("first") {
		t.Fatal("credential scope must be stable")
	}
	t.Setenv("ORCHESTRA_STATE_FILE", "/tmp/isolated-state.json")
	if first == linearKeyService("first") {
		t.Fatal("QA state must not access the user's credentials")
	}
}
