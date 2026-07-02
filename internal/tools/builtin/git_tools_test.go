package builtin

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a git repo in dir with an initial commit, then
// returns. Fails the test if git is unavailable.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	// Require git; skip the test entirely if not installed.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping git tool tests")
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	// Initial commit so there's a HEAD.
	writeFile(t, dir, ".gitkeep", "")
	run("add", "-A")
	run("commit", "-m", "init")
}

func TestGitStatus_Clean(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	g := GitStatus{Ctx: ctx}
	out, err := g.Execute(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "working tree clean" {
		t.Errorf("got %q, want 'working tree clean'", out)
	}
}

func TestGitStatus_Dirty(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	writeFile(t, dir, "new.txt", "content")
	g := GitStatus{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{}`))
	if !strings.Contains(out, "new.txt") {
		t.Errorf("expected new.txt in status, got %q", out)
	}
}

func TestGitDiff_Unstaged(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	writeFile(t, dir, "file.txt", "original\n")
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "add file").Run()
	writeFile(t, dir, "file.txt", "modified\n")

	g := GitDiff{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{}`))
	if !strings.Contains(out, "-original") || !strings.Contains(out, "+modified") {
		t.Errorf("expected diff output, got %q", out)
	}
}

func TestGitDiff_NoChanges(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	g := GitDiff{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{}`))
	if out != "no changes" {
		t.Errorf("got %q, want 'no changes'", out)
	}
}

func TestGitCommit_CreatesCommit(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	writeFile(t, dir, "feature.txt", "new feature")
	g := GitCommit{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{"message":"add feature"}`))
	if !strings.Contains(out, "committed") {
		t.Errorf("got %q", out)
	}
	// Verify: git log should show the commit.
	logOut, _ := exec.Command("git", "-C", dir, "log", "--oneline").Output()
	if !strings.Contains(string(logOut), "add feature") {
		t.Errorf("commit not in log:\n%s", logOut)
	}
	// Working tree should now be clean.
	statusOut, _ := exec.Command("git", "-C", dir, "status", "--short").Output()
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Errorf("expected clean tree after commit, got:\n%s", statusOut)
	}
}

func TestGitCommit_MissingMessage(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	g := GitCommit{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{"message":""}`))
	if !strings.Contains(out, "message is required") {
		t.Errorf("got %q", out)
	}
}

func TestGitDiff_PathScoped(t *testing.T) {
	ctx, dir := ctxFor(t)
	initGitRepo(t, dir)
	writeFile(t, dir, "a.txt", "original\n")
	writeFile(t, dir, "b.txt", "original\n")
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init files").Run()
	writeFile(t, dir, "a.txt", "modified\n")
	writeFile(t, dir, "b.txt", "modified\n")

	g := GitDiff{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{"path":"a.txt"}`))
	if !strings.Contains(out, "a.txt") {
		t.Errorf("expected a.txt in diff:\n%s", out)
	}
	if strings.Contains(out, "b.txt") {
		t.Errorf("b.txt should be excluded by path scope:\n%s", out)
	}
}

func TestGit_NotARepo(t *testing.T) {
	ctx, _ := ctxFor(t)
	// No git init.
	g := GitStatus{Ctx: ctx}
	out, _ := g.Execute(json.RawMessage(`{}`))
	if !strings.HasPrefix(out, "Error:") {
		t.Errorf("expected error for non-repo, got %q", out)
	}
}

// silence unused import warning
var _ = filepath.Join
