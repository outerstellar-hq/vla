package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/abrandt/vla/internal/fsutil"
)

// gitCommon runs `git <args...>` in BaseDir and returns trimmed stdout.
// On failure returns the trimmed stderr as an error string (suitable for
// Tool.Execute's result string convention).
func gitCommon(baseDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = baseDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := string(bytes.TrimSpace(stderr.Bytes()))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", errMsg)
	}
	return string(bytes.TrimSpace(stdout.Bytes())), nil
}

// GitStatus returns `git status --short` output.
type GitStatus struct{ Ctx Ctx }

func (GitStatus) Name() string { return "git_status" }

func (GitStatus) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"porcelain": map[string]any{
				"type":        "boolean",
				"description": "If true (default), return short porcelain format. If false, return human-readable.",
			},
		},
	}
}

func (g GitStatus) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Porcelain *bool `json:"porcelain"`
	}
	_ = json.Unmarshal(args, &in)
	format := "--short"
	if in.Porcelain != nil && !*in.Porcelain {
		format = ""
	}
	cmdArgs := []string{"status"}
	if format != "" {
		cmdArgs = append(cmdArgs, format)
	}
	out, err := gitCommon(g.Ctx.BaseDir, cmdArgs...)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if out == "" {
		return "working tree clean", nil
	}
	return out, nil
}

// GitDiff returns `git diff` output (unstaged by default, or staged).
type GitDiff struct{ Ctx Ctx }

func (GitDiff) Name() string { return "git_diff" }

func (GitDiff) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"staged": map[string]any{
				"type":        "boolean",
				"description": "If true, show staged (--cached) changes. Default false (unstaged).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional path to limit the diff to.",
			},
		},
	}
}

func (g GitDiff) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Staged bool   `json:"staged"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	cmdArgs := []string{"diff"}
	if in.Staged {
		cmdArgs = append(cmdArgs, "--cached")
	}
	if in.Path != "" {
		// Confine the path argument; git takes paths relative to cwd.
		abs, err := fsutil.Confine(g.Ctx.BaseDir, in.Path)
		if err != nil {
			return "Error: " + err.Error(), nil
		}
		rel, err := filepath.Rel(g.Ctx.BaseDir, abs)
		if err != nil {
			return "Error: " + err.Error(), nil
		}
		cmdArgs = append(cmdArgs, "--", rel)
	}
	out, err := gitCommon(g.Ctx.BaseDir, cmdArgs...)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if out == "" {
		return "no changes", nil
	}
	return out, nil
}

// GitCommit stages all changes and creates a commit.
type GitCommit struct{ Ctx Ctx }

func (GitCommit) Name() string { return "git_commit" }

func (GitCommit) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The commit message.",
			},
			"all": map[string]any{
				"type":        "boolean",
				"description": "If true (default), stage all tracked changes (-a) before committing.",
			},
		},
		"required": []string{"message"},
	}
}

func (g GitCommit) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Message string `json:"message"`
		All     *bool  `json:"all"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Message == "" {
		return "Error: message is required", nil
	}
	stageAll := true
	if in.All != nil {
		stageAll = *in.All
	}
	if stageAll {
		if _, err := gitCommon(g.Ctx.BaseDir, "add", "-A"); err != nil {
			return "Error: git add: " + err.Error(), nil
		}
	}
	if _, err := gitCommon(g.Ctx.BaseDir, "commit", "-m", in.Message); err != nil {
		return "Error: git commit: " + err.Error(), nil
	}
	// Return the new HEAD as confirmation.
	hash, err := gitCommon(g.Ctx.BaseDir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return fmt.Sprintf("committed: %s", in.Message), nil
	}
	return fmt.Sprintf("committed %s: %s", hash, in.Message), nil
}
