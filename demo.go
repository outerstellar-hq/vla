// demo.go — the `vla demo` subcommand: renders TUI scenes as ANSI art for
// screenshot generation. No API keys, no LLM calls, no network — fully
// deterministic. Output is piped to aha + wkhtmltoimage by the CI pipeline.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/abrandt/vla/internal/tui"
)

func runDemoCmd(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	outDir := fs.String("out", ".", "output directory for .ansi files")
	width := fs.Int("width", 100, "terminal width")
	height := fs.Int("height", 30, "terminal height")
	gifMode := fs.Bool("gif", false, "generate animated GIF frames instead of static scenes")
	fs.Parse(args)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "vla demo: cannot create output dir: %v\n", err)
		os.Exit(1)
	}

	opts := tui.DemoOptions{
		ModelName:  "gpt-4o",
		SessionID:  "20260704T120000Z",
		ToolCount:  24,
		Tokens:     15234,
		Width:      *width,
		Height:     *height,
		Spinning:   false,
		StatusText: "idle",
	}

	if *gifMode {
		sceneGIF(*outDir)
		return
	}

	// Scene 1: Main conversation with tool calls.
	scene1(opts, *outDir)

	// Scene 2: Split-pane diff view.
	scene2(opts, *outDir)

	// Scene 3: Session picker.
	scene3(opts, *outDir)

	fmt.Fprintf(os.Stderr, "vla demo: wrote 3 scenes to %s\n", *outDir)
}

// sceneGIF renders the default demo sequence as individual ANSI frames
// in a frames/ subdirectory. Each frame is numbered (frame-001.ansi,
// frame-002.ansi, etc.) for ordered GIF assembly.
func sceneGIF(outDir string) {
	framesDir := filepath.Join(outDir, "frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "vla demo gif: cannot create frames dir: %v\n", err)
		os.Exit(1)
	}

	frames := tui.DefaultDemoSequence()
	for i, frame := range frames {
		path := filepath.Join(framesDir, fmt.Sprintf("frame-%03d.ansi", i+1))
		writeAnsi(path, frame.RenderSingle())
	}

	fmt.Fprintf(os.Stderr, "vla demo gif: wrote %d frames to %s\n", len(frames), framesDir)
	fmt.Fprintf(os.Stderr, "  convert: bash scripts/frames-to-gif.sh %s assets/demo.gif\n", framesDir)
}

// scene1 renders a conversation with a user message, an assistant response
// with markdown, and two completed tool calls.
func scene1(opts tui.DemoOptions, outDir string) {
	streamingOpts := opts
	streamingOpts.Spinning = true
	streamingOpts.StatusText = "thinking"
	streamingOpts.Streaming = "I'll look at the authentication module to understand how the login flow works."

	blocks := []tui.DemoBlock{
		{
			Type:    "user",
			Content: "Fix the login bug in auth.py — users can't log in with valid credentials",
		},
		{
			Type:       "tool",
			ToolName:   "read_file",
			ToolArgs:   `{"path":"auth.py"}`,
			ToolResult: "import hashlib\nfrom db import get_user\n\ndef login(username, password):\n    user = get_user(username)\n    if user and verify(password, user.hash):\n        return create_session(user)\n    return None\n\ndef verify(password, stored_hash):\n    return hashlib.md5(password.encode()).hexdigest() == stored_hash",
			Status:     "done",
		},
		{
			Type:       "tool",
			ToolName:   "search",
			ToolArgs:   `{"query":"verify","pattern":"*.py"}`,
			ToolResult: "auth.py:10:def verify(password, stored_hash):\nauth.py:11:    return hashlib.md5(password.encode()).hexdigest() == stored_hash\ntests/test_auth.py:5:from auth import verify",
			Status:     "done",
		},
	}

	frame := tui.RenderDemoFrame(blocks, streamingOpts)
	writeAnsi(filepath.Join(outDir, "demo-conversation.ansi"), frame)
}

// scene2 renders the split-pane diff view for an update_file call.
func scene2(opts tui.DemoOptions, outDir string) {
	diffOpts := opts
	diffOpts.Spinning = true
	diffOpts.StatusText = "running: update_file"

	blocks := []tui.DemoBlock{
		{
			Type:    "user",
			Content: "The verify function uses MD5 — that's insecure. Switch to bcrypt.",
		},
		{
			Type:    "assistant",
			Content: "You're right — MD5 is cryptographically broken. I'll update `verify()` to use bcrypt with a proper salt.",
		},
		{
			Type:     "tool",
			ToolName: "update_file",
			ToolArgs: `{"path":"auth.py","old_string":"def verify(password, stored_hash):\n    return hashlib.md5(password.encode()).hexdigest() == stored_hash","new_string":"import bcrypt\n\ndef verify(password, stored_hash):\n    return bcrypt.checkpw(password.encode(), stored_hash.encode())"}`,
			Status:   "running",
		},
	}

	diffTool := "update_file"
	diffArgs := `{"path":"auth.py","old_string":"def verify(password, stored_hash):\n    return hashlib.md5(password.encode()).hexdigest() == stored_hash","new_string":"import bcrypt\n\ndef verify(password, stored_hash):\n    return bcrypt.checkpw(password.encode(), stored_hash.encode())"}`

	frame := tui.RenderDemoDiff(blocks, diffTool, diffArgs, diffOpts)
	writeAnsi(filepath.Join(outDir, "demo-diff.ansi"), frame)
}

// scene3 renders the session picker with sample sessions.
func scene3(opts tui.DemoOptions, outDir string) {
	items := []tui.SessionItem{
		{ID: "20260704T120000Z", Project: "/home/dev/vla", Model: "gpt-4o", Created: time.Now().Add(-30 * time.Minute)},
		{ID: "20260704T090000Z", Project: "/home/dev/vla", Model: "gpt-4o", Created: time.Now().Add(-4 * time.Hour)},
		{ID: "20260703T160000Z", Project: "/home/dev/vla", Model: "claude-3.5-sonnet", Created: time.Now().Add(-20 * time.Hour)},
		{ID: "20260702T110000Z", Project: "/home/dev/vla", Model: "gpt-4o", Created: time.Now().Add(-48 * time.Hour)},
		{ID: "20260701T140000Z", Project: "/home/dev/vla", Model: "gpt-4o-mini", Created: time.Now().Add(-72 * time.Hour)},
	}

	frame := tui.RenderDemoPicker(items, opts)
	writeAnsi(filepath.Join(outDir, "demo-sessions.ansi"), frame)
}

// writeAnsi writes an ANSI frame to a file.
func writeAnsi(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "vla demo: cannot write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  wrote %s (%d bytes)\n", path, len(content))
}
