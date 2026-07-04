// bug_report.go — the `vla bug` subcommand: creates a GitHub issue directly
// in the VLA repo (outerstellar-hq/vla). Uses the gh CLI if available,
// falls back to the GitHub API with a token. No third-party routing.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const githubRepo = "outerstellar-hq/vla"

func runBugCmd(args []string) {
	title, body := parseBugArgs(args)

	// If no title provided, prompt interactively.
	if title == "" {
		title = promptInput("Bug title (one line): ")
		if title == "" {
			fmt.Fprintln(os.Stderr, "vla bug: title is required")
			os.Exit(1)
		}
	}

	if body == "" {
		body = promptMultiline("Describe the bug (Ctrl+D to finish): ")
	}

	// Append environment info to the body.
	body = appendEnvironment(body)

	// Try gh CLI first.
	if hasGhCLI() {
		createIssueWithGh(title, body)
		return
	}

	// Fall back to GitHub API.
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "vla bug: no GitHub authentication found")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "To report a bug, either:")
		fmt.Fprintln(os.Stderr, "  1. Install the GitHub CLI: https://cli.github.com")
		fmt.Fprintln(os.Stderr, "  2. Set GITHUB_TOKEN to a personal access token")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Or report directly at:")
		fmt.Fprintf(os.Stderr, "  https://github.com/%s/issues/new\n", githubRepo)
		os.Exit(1)
	}

	createIssueWithAPI(token, title, body)
}

// parseBugArgs extracts --title and --body from args.
func parseBugArgs(args []string) (title, body string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 < len(args) {
				title = args[i+1]
				i++
			}
		case "--body":
			if i+1 < len(args) {
				body = args[i+1]
				i++
			}
		default:
			if title == "" {
				title = args[i]
			}
		}
	}
	return
}

// hasGhCLI returns true if the gh CLI is on PATH and authenticated.
func hasGhCLI() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// createIssueWithGh uses the GitHub CLI to create an issue.
func createIssueWithGh(title, body string) {
	args := []string{
		"issue", "create",
		"--repo", githubRepo,
		"--title", title,
		"--body", body,
		"--label", "bug",
	}

	cmd := exec.Command("gh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla bug: gh failed: %v\n", err)
		os.Exit(1)
	}

	url := strings.TrimSpace(string(output))
	fmt.Printf("Bug reported: %s\n", url)
}

// createIssueWithAPI creates a GitHub issue via the REST API.
func createIssueWithAPI(token, title, body string) {
	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
	})

	url := fmt.Sprintf("https://api.github.com/repos/%s/issues", githubRepo)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla bug: API request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 201 {
		fmt.Fprintf(os.Stderr, "vla bug: GitHub API returned %d\n", resp.StatusCode)
		fmt.Fprintf(os.Stderr, "  %s\n", string(respBody))
		os.Exit(1)
	}

	var result struct {
		HTMLURL string `json:"html_url"`
	}
	json.Unmarshal(respBody, &result)

	fmt.Printf("Bug reported: %s\n", result.HTMLURL)
}

// appendEnvironment adds OS/runtime info to the bug body.
func appendEnvironment(body string) string {
	var b strings.Builder
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n---\n")
	b.WriteString(fmt.Sprintf("**VLA version:** %s\n", version))
	b.WriteString(fmt.Sprintf("**OS:** %s/%s\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("**Go version:** %s\n", runtime.Version()))
	return b.String()
}

// promptInput reads a single line from stdin.
func promptInput(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// promptMultiline reads multiple lines until EOF (Ctrl+D) or a blank line.
func promptMultiline(prompt string) string {
	fmt.Fprintln(os.Stderr, prompt)
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && len(lines) > 0 {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
