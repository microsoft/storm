package libvirt

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestCtxWriterStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	w := ctxWriter{ctx, &buf}
	if _, err := w.Write([]byte("one")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	cancel()
	if _, err := w.Write([]byte("two")); !errors.Is(err, context.Canceled) {
		t.Fatalf("write after cancel = %v, want context.Canceled", err)
	}
	if buf.String() != "one" {
		t.Errorf("buffer = %q, want %q", buf.String(), "one")
	}
}

func TestStartCaptureNoPathIsNoop(t *testing.T) {
	c := &Client{}
	c.caps.init()
	defer c.caps.shutdown()
	if err := c.startCapture("ns_vm", ""); err != nil {
		t.Fatalf("startCapture: %v", err)
	}
	if len(c.caps.m) != 0 {
		t.Errorf("capture registered without a path: %v", c.caps.m)
	}
}

func TestStartCaptureAfterClose(t *testing.T) {
	c := &Client{}
	c.caps.init()
	c.caps.shutdown()
	if err := c.startCapture("ns_vm", "/nonexistent/serial.log"); err == nil {
		t.Fatal("startCapture on a closed client succeeded")
	}
}

func TestStopCapturesWaitsAndHonoursContext(t *testing.T) {
	c := &Client{}
	c.caps.init()
	done := make(chan struct{})
	cancelled := false
	c.caps.m["a"] = &capture{cancel: func() { cancelled = true }, done: done}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.stopCaptures(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("stopCaptures with stuck capture = %v, want context.Canceled", err)
	}
	if !cancelled {
		t.Error("capture was not cancelled")
	}

	close(done)
	if err := c.stopCaptures(context.Background()); err != nil {
		t.Fatalf("stopCaptures after finish: %v", err)
	}
}
