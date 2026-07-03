package tui

import (
	"io"
	"testing"
)

func TestChannelInput_SendAndReceive(t *testing.T) {
	c := NewChannelInput()
	go func() {
		c.Send("hello world")
	}()
	text, err := c.Readline()
	if err != nil {
		t.Fatalf("Readline: %v", err)
	}
	if text != "hello world" {
		t.Errorf("got %q", text)
	}
}

func TestChannelInput_CloseReturnsEOF(t *testing.T) {
	c := NewChannelInput()
	go func() {
		c.Close()
	}()
	_, err := c.Readline()
	if err != io.EOF {
		t.Errorf("expected EOF after close, got %v", err)
	}
}

func TestChannelWriter_WriteAndRead(t *testing.T) {
	w := NewChannelWriter()
	go func() {
		w.Write([]byte("streaming token"))
	}()
	got := <-w.Chan()
	if got != "streaming token" {
		t.Errorf("got %q", got)
	}
}

func TestChannelWriter_MultipleWrites(t *testing.T) {
	w := NewChannelWriter()
	go func() {
		w.Write([]byte("a"))
		w.Write([]byte("b"))
		w.Write([]byte("c"))
	}()
	var got string
	for i := 0; i < 3; i++ {
		got += <-w.Chan()
	}
	if got != "abc" {
		t.Errorf("got %q, want abc", got)
	}
}
