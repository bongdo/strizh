package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPositionUTF16(t *testing.T) {
	src := []byte("a😀b\nsecond")
	p := PositionAt(src, len("a😀b"))
	if p.Line != 0 || p.Character != 4 {
		t.Fatalf("got %+v", p)
	}
	if off := OffsetAt(src, Position{Line: 0, Character: 3}); off != len("a😀") {
		t.Fatalf("offset %d", off)
	}
	if off := OffsetAt(src, Position{Line: 1, Character: 2}); off != len("a😀b\nse") {
		t.Fatalf("offset %d", off)
	}
}

func TestConnRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	c := NewConn(nil, &buf)
	if err := c.Notify("test/notify", map[string]int{"x": 1}); err != nil {
		t.Fatal(err)
	}
	msg, err := NewConn(&buf, nil).Read()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Method != "test/notify" || !strings.Contains(string(msg.Params), `"x":1`) {
		t.Fatalf("%+v", msg)
	}
}

func TestConnReply(t *testing.T) {
	var buf bytes.Buffer
	c := NewConn(nil, &buf)
	if err := c.Reply(json.RawMessage(`1`), map[string]string{"ok": "yes"}); err != nil {
		t.Fatal(err)
	}
	msg, err := NewConn(&buf, nil).Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg.ID) != "1" || !strings.Contains(string(msg.Result), `"ok":"yes"`) {
		t.Fatalf("%+v", msg)
	}
}
