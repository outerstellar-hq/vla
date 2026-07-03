package tui

import "io"

// ChannelInput implements agent.InputReader by reading from a channel.
// The TUI sends submitted text here; the loop reads it as the next message.
type ChannelInput struct {
	Ch chan string
}

func NewChannelInput() *ChannelInput {
	return &ChannelInput{Ch: make(chan string)}
}

func (c *ChannelInput) Send(text string) { c.Ch <- text }
func (c *ChannelInput) Readline() (string, error) {
	text, ok := <-c.Ch
	if !ok {
		return "", io.EOF
	}
	return text, nil
}
func (c *ChannelInput) Close() error { close(c.Ch); return nil }

// ChannelWriter implements io.Writer by sending written bytes to a channel
// as strings. The agent loop's streaming output goes here; the TUI reads
// it and updates the conversation pane in real-time.
type ChannelWriter struct {
	Ch chan string
}

func NewChannelWriter() *ChannelWriter {
	return &ChannelWriter{Ch: make(chan string, 256)}
}

func (w *ChannelWriter) Write(p []byte) (int, error) {
	w.Ch <- string(p)
	return len(p), nil
}

func (w *ChannelWriter) Chan() chan string { return w.Ch }
