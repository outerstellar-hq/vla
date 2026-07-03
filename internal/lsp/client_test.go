package lsp

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pipeClient creates a client wired to an in-memory pipe pair, so we can
// simulate an LSP server's responses without a real process.
type pipeConn struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (p *pipeConn) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *pipeConn) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeConn) Close() error                { p.r.Close(); return p.w.Close() }

// newPipePair creates two connected pipe-based read/write pairs.
// serverRead ← client writes; client reads ← server writes.
func newPipePair() (clientIn io.Reader, clientOut io.Writer, serverIn io.Reader, serverOut io.Writer) {
	sr1, sw1 := io.Pipe() // client writes → server reads
	sr2, sw2 := io.Pipe() // server writes → client reads
	return sr2, sw1, sr1, sw2
}

// writeLSPMessage writes a Content-Length framed message to w.
func writeLSPMessage(w io.Writer, msg []byte) error {
	header := "Content-Length: " + intToStr(len(msg)) + "\r\n\r\n"
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}

func intToStr(n int) string {
	return strings.TrimSpace(strings.Replace(" "+itoa(int64(n)), " ", "", 1))
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// TestClient_RequestResponse simulates a full request-response cycle through
// the pipe pair: the test acts as the "server" writing a response.
func TestClient_RequestResponse(t *testing.T) {
	clientIn, clientOut, _, serverOut := newPipePair()
	defer clientOut.(io.Closer).Close()
	defer serverOut.(io.Closer).Close()

	c := NewClient(clientIn, clientOut)
	c.Start()
	defer c.Close()

	// Act as server: read the request, send a response.
	go func() {
		// Read the request (we don't parse it fully, just need the id).
		buf := make([]byte, 4096)
		// Read headers + body from serverIn — but we have serverOut here.
		// Actually we need to read from the pipe the client writes to.
		// clientOut is sw1; server reads from sr1. We don't have sr1 here.
		// Let me restructure...
		_ = buf
	}()

	// This test structure is too convoluted with pipes. Let me use a
	// simpler approach: net.Pipe gives us a duplex connection.
	t.Skip("restructured below")
}

func TestClient_NetPipe_RequestResponse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	c := NewClient(clientConn, clientConn)
	c.Start()
	defer c.Close()

	// Server goroutine: read request, echo back a response with the same id.
	go func() {
		defer serverConn.Close()
		// Read one message from the client.
		msg, err := readMessageRaw(serverConn)
		if err != nil {
			return
		}
		var req map[string]any
		json.Unmarshal(msg, &req)
		id := req["id"]

		// Send a response.
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  map[string]any{"uri": "file:///test.go", "range": map[string]any{}},
		}
		respBytes, _ := json.Marshal(resp)
		writeLSPMessage(serverConn, respBytes)
	}()

	result, err := c.Request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": "file:///test.go"},
		"position":     map[string]any{"line": 0, "character": 0},
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	var def struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(result, &def); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if def.URI != "file:///test.go" {
		t.Errorf("uri = %q", def.URI)
	}
}

func TestClient_NetPipe_Error(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	c := NewClient(clientConn, clientConn)
	c.Start()
	defer c.Close()

	go func() {
		defer serverConn.Close()
		msg, _ := readMessageRaw(serverConn)
		var req map[string]any
		json.Unmarshal(msg, &req)
		id := req["id"]

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error":   map[string]any{"code": -32601, "message": "method not found"},
		}
		respBytes, _ := json.Marshal(resp)
		writeLSPMessage(serverConn, respBytes)
	}()

	_, err := c.Request("unknown/method", nil)
	if err == nil {
		t.Fatal("expected error response")
	}
	if !strings.Contains(err.Error(), "method not found") {
		t.Errorf("error = %v", err)
	}
}

func TestClient_NetPipe_Notification(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	c := NewClient(clientConn, clientConn)
	c.Start()
	defer c.Close()

	received := make(chan json.RawMessage, 1)
	c.OnNotification("textDocument/publishDiagnostics", func(params json.RawMessage) {
		received <- params
	})

	go func() {
		defer serverConn.Close()
		notif := map[string]any{
			"jsonrpc": "2.0",
			"method":  "textDocument/publishDiagnostics",
			"params": map[string]any{
				"uri":         "file:///test.go",
				"diagnostics": []any{},
			},
		}
		notifBytes, _ := json.Marshal(notif)
		writeLSPMessage(serverConn, notifBytes)
		time.Sleep(100 * time.Millisecond)
	}()

	select {
	case params := <-received:
		var p struct {
			URI string `json:"uri"`
		}
		json.Unmarshal(params, &p)
		if p.URI != "file:///test.go" {
			t.Errorf("uri = %q", p.URI)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive notification within 1s")
	}
}

func TestClient_DiagnosticsBuffered(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	c := NewClient(clientConn, clientConn)
	c.Start()
	defer c.Close()

	go func() {
		defer serverConn.Close()
		notif := map[string]any{
			"jsonrpc": "2.0",
			"method":  "textDocument/publishDiagnostics",
			"params": map[string]any{
				"uri":         "file:///x.go",
				"diagnostics": []any{map[string]any{"message": "undefined: foo"}},
			},
		}
		b, _ := json.Marshal(notif)
		writeLSPMessage(serverConn, b)
		time.Sleep(200 * time.Millisecond)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if c.GetDiagnostics("file:///x.go") != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if c.GetDiagnostics("file:///x.go") == nil {
		t.Fatal("diagnostics not buffered")
	}
}

func TestPathToURI(t *testing.T) {
	got := pathToURI("/home/user/project")
	if !strings.HasPrefix(got, "file://") {
		t.Errorf("uri should start with file://, got %q", got)
	}
}

func TestInferLanguage_GoMod(t *testing.T) {
	// This test creates a temp dir with go.mod and checks inference.
	dir := t.TempDir()
	// Write a fake go.mod.
	writeTestFile(t, dir, "go.mod", "module test\n")
	lang := InferLanguage(dir)
	if lang != LangGo {
		t.Errorf("expected LangGo, got %q", lang)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// readMessageRaw reads a Content-Length framed message from r without using
// bufio (for use with net.Pipe).
func readMessageRaw(r io.Reader) ([]byte, error) {
	// Read headers byte-by-byte until \r\n\r\n.
	buf := make([]byte, 1)
	var headers string
	for {
		_, err := r.Read(buf)
		if err != nil {
			return nil, err
		}
		headers += string(buf)
		if strings.HasSuffix(headers, "\r\n\r\n") {
			break
		}
		if len(headers) > 4096 {
			return nil, io.ErrShortBuffer
		}
	}
	// Parse content-length.
	contentLength := 0
	for _, line := range strings.Split(headers, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			val := strings.TrimSpace(line[15:])
			contentLength = atoiSimple(val)
		}
	}
	if contentLength <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	return body, err
}

func atoiSimple(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
