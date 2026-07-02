package builtin

import "testing"

func TestEcho_Execute(t *testing.T) {
	var e Echo
	got, err := e.Execute([]byte(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestEcho_Execute_MissingText(t *testing.T) {
	var e Echo
	got, _ := e.Execute([]byte(`{}`))
	if got != "Error: text is required" {
		t.Errorf("got %q, want error string", got)
	}
}

func TestEcho_Execute_MalformedJSON(t *testing.T) {
	var e Echo
	got, err := e.Execute([]byte(`{not json`))
	if err != nil {
		t.Fatalf("tool errors should be returned as result strings, not Go errors; got: %v", err)
	}
	if got == "" || got[:5] != "Error" {
		t.Errorf("got %q, want an error string starting with 'Error'", got)
	}
}
