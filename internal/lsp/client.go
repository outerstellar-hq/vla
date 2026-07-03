// Package lsp implements a minimal LSP (Language Server Protocol) client
// for VLA. It communicates with language servers (pyright, gopls, etc.)
// over stdio using the LSP base protocol: headers + Content-Length framing.
//
// This is the real "ctrl+click" engine — when an LSP server is available for
// the project's language, navigation tools (go-to-definition, find-references,
// hover) query it instead of the regex-based indexer. The regex indexer
// remains as a fallback.
//
// Architecture mirrors Memwizard's LSP layer:
//   - Client: JSON-RPC over Content-Length framing
//   - Manager: warm process pool, one server per (language, workspace)
//   - Tools call the Manager, which returns a Client for the right language
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Client is a JSON-RPC client that speaks the LSP base protocol over an
// io.Reader (server stdout) and io.Writer (server stdin). It is safe for
// concurrent use.
type Client struct {
	r       io.Reader
	w       io.Writer
	writeMu sync.Mutex

	id      atomic.Int64
	pending sync.Map // map[int64]chan *rpcResponse

	notifications   sync.Map // map[string][]chan json.RawMessage — server→client notifications
	diagnostics     sync.Map // map[string]json.RawMessage — latest publishDiagnostics per URI
	diagnosticsLock sync.RWMutex

	done chan struct{}
}

// rpcResponse is the JSON-RPC response envelope.
type rpcResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// NewClient wraps the given reader/writer pair into an LSP client. Call
// Start() to begin the read loop.
func NewClient(r io.Reader, w io.Writer) *Client {
	c := &Client{r: r, w: w, done: make(chan struct{})}
	return c
}

// Start begins reading from the server's stdout in a background goroutine.
// Responses and notifications are dispatched to waiters/callbacks.
func (c *Client) Start() {
	go c.readLoop()
}

// Close signals the read loop to stop.
func (c *Client) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

// Request sends a JSON-RPC request and waits for the response. The result
// is returned as raw JSON; the caller unmarshals into the expected type.
func (c *Client) Request(method string, params any) (json.RawMessage, error) {
	id := c.id.Add(1)

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("lsp: marshal request: %w", err)
	}

	ch := make(chan *rpcResponse, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	if err := c.writeMessage(msgBytes); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-c.done:
		return nil, fmt.Errorf("lsp: client closed while waiting for %s", method)
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("lsp: marshal notification: %w", err)
	}
	return c.writeMessage(msgBytes)
}

// OnNotification registers a handler for a server notification of the given
// method. Returns an unregistration function.
func (c *Client) OnNotification(method string, handler func(json.RawMessage)) func() {
	ch := make(chan json.RawMessage, 16)
	val, _ := c.notifications.LoadOrStore(method, []chan json.RawMessage{ch})
	chans := val.([]chan json.RawMessage)
	if len(chans) > 0 && chans[0] != ch {
		// LoadOrStore returned existing; append.
		chans = append(chans, ch)
		c.notifications.Store(method, chans)
	}
	go func() {
		for {
			select {
			case <-c.done:
				return
			case raw, ok := <-ch:
				if !ok {
					return
				}
				handler(raw)
			}
		}
	}()
	return func() {
		// Best-effort removal.
		val, ok := c.notifications.Load(method)
		if !ok {
			return
		}
		chans := val.([]chan json.RawMessage)
		filtered := chans[:0]
		for _, ch2 := range chans {
			if ch2 != ch {
				filtered = append(filtered, ch2)
			}
		}
		c.notifications.Store(method, filtered)
		close(ch)
	}
}

// GetDiagnostics returns the latest diagnostics for a URI (from
// textDocument/publishDiagnostics notifications).
func (c *Client) GetDiagnostics(uri string) json.RawMessage {
	c.diagnosticsLock.RLock()
	defer c.diagnosticsLock.RUnlock()
	if v, ok := c.diagnostics.Load(uri); ok {
		return v.(json.RawMessage)
	}
	return nil
}

// writeMessage frames and writes one LSP message.
func (c *Client) writeMessage(msg []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
	if _, err := io.WriteString(c.w, header); err != nil {
		return fmt.Errorf("lsp: write header: %w", err)
	}
	if _, err := c.w.Write(msg); err != nil {
		return fmt.Errorf("lsp: write body: %w", err)
	}
	return nil
}

// readLoop continuously reads and dispatches LSP messages from the server.
func (c *Client) readLoop() {
	reader := bufio.NewReader(c.r)
	for {
		select {
		case <-c.done:
			return
		default:
		}
		msg, err := readMessage(reader)
		if err != nil {
			// EOF or error — fail all pending requests.
			c.pending.Range(func(key, val any) bool {
				ch := val.(chan *rpcResponse)
				select {
				case ch <- &rpcResponse{Error: &rpcError{Code: -1, Message: "connection closed"}}:
				default:
				}
				return true
			})
			return
		}

		// Determine if this is a response (has "id") or notification (no "id").
		var envelope struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue // skip malformed
		}

		if envelope.ID != nil && envelope.Method == "" {
			// It's a response — dispatch to the waiter.
			if val, ok := c.pending.Load(*envelope.ID); ok {
				ch := val.(chan *rpcResponse)
				ch <- &rpcResponse{
					ID:     *envelope.ID,
					Result: envelope.Result,
					Error:  envelope.Error,
				}
			}
		} else if envelope.Method != "" {
			// It's a notification from the server.
			c.dispatchNotification(envelope.Method, envelope.Params)
		}
	}
}

func (c *Client) dispatchNotification(method string, params json.RawMessage) {
	if method == "textDocument/publishDiagnostics" {
		var diagParams struct {
			URI         string          `json:"uri"`
			Diagnostics json.RawMessage `json:"diagnostics"`
		}
		if err := json.Unmarshal(params, &diagParams); err == nil {
			c.diagnosticsLock.Lock()
			c.diagnostics.Store(diagParams.URI, diagParams.Diagnostics)
			c.diagnosticsLock.Unlock()
		}
	}
	val, ok := c.notifications.Load(method)
	if !ok {
		return
	}
	for _, ch := range val.([]chan json.RawMessage) {
		select {
		case ch <- params:
		default: // drop if handler is slow
		}
	}
}

// readMessage reads one LSP base-protocol message (headers + body) from r.
func readMessage(r *bufio.Reader) ([]byte, error) {
	// Read headers until empty line.
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			val := strings.TrimSpace(line[len("Content-Length:"):])
			contentLength, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("lsp: parse content-length: %w", err)
			}
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("lsp: no content-length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
