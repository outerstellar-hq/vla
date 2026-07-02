package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abrandt/vla/internal/tools"
)

func TestResolveConfigPath_ExplicitWins(t *testing.T) {
	// When an explicit path is given, it is returned verbatim — even if other
	// candidates exist on disk.
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Chdir(dir)
	// Create ./config.json in the temp dir so we can prove explicit overrides it.
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644)

	got := ResolveConfigPath("/explicit/path.json")
	if got != "/explicit/path.json" {
		t.Errorf("explicit = %q, want /explicit/path.json", got)
	}
	_ = cwd
}

func TestResolveConfigPath_CWDPriority(t *testing.T) {
	// When no explicit path and ./config.json exists in CWD, it wins over the
	// home fallback.
	dir := t.TempDir()
	t.Chdir(dir)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644)

	got := ResolveConfigPath("")
	if got != "config.json" {
		t.Errorf("cwd fallback = %q, want config.json", got)
	}
}

func TestResolveConfigPath_HomeFallback(t *testing.T) {
	// When no explicit and no ./config.json, returns ~/.vla/config.json.
	dir := t.TempDir()
	t.Chdir(dir)

	got := ResolveConfigPath("")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".vla", "config.json")
	if got != want {
		t.Errorf("home fallback = %q, want %q", got, want)
	}
}

func TestRegisterBuiltins_RegistersAll(t *testing.T) {
	r := tools.NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	// Every tool listed in RegisterBuiltins must be findable by name.
	// Update this list when new builtins are added.
	wantNames := []string{"echo"}
	for _, name := range wantNames {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected tool %q registered, not found", name)
		}
	}

	schemas := r.Schemas()
	if len(schemas) != len(wantNames) {
		t.Errorf("expected %d schemas, got %d", len(wantNames), len(schemas))
	}
}

func TestRegisterBuiltins_EchoSchemaValid(t *testing.T) {
	// The echo tool's schema must have the shape the OpenAI API expects.
	r := tools.NewRegistry()
	_ = RegisterBuiltins(r)

	_, ok := r.Get("echo")
	if !ok {
		t.Fatal("echo not registered")
	}
	// Schema is validated structurally: the registry wraps it as
	// {"type":"function","function":{"name":"echo","parameters":{...}}}.
	schema := r.Schemas()[0]
	fn, ok := schema["function"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing function wrapper: %v", schema)
	}
	if fn["name"] != "echo" {
		t.Errorf("function.name = %v, want echo", fn["name"])
	}
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("missing parameters: %v", fn)
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type = %v, want object", params["type"])
	}
}

func TestOpenOrCreateSession_NewCreatesTranscript(t *testing.T) {
	dir := t.TempDir()
	// Override the sessions dir by setting HOME (UserHomeDir reads $HOME on
	// unix, $USERPROFILE on windows). t.Setenv scopes the change to this test.
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	sess, err := OpenOrCreateSession("", "test-model")
	if err != nil {
		t.Fatalf("OpenOrCreateSession new: %v", err)
	}
	if sess.ID() == "" {
		t.Fatal("session ID is empty")
	}

	// The transcript file must exist.
	path := filepath.Join(dir, ".vla", "sessions", sess.ID()+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("transcript not created at %s: %v", path, err)
	}
}
