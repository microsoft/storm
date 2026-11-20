package ssh

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

type blockedErrorWriter struct {
	started chan struct{}
	release chan struct{}
}

func (w blockedErrorWriter) Write([]byte) (int, error) {
	close(w.started)
	<-w.release
	return 0, &os.PathError{Op: "write", Path: "artifact", Err: io.ErrClosedPipe}
}

func TestWaitCollectsWriteErrorAfterCancellation(t *testing.T) {
	writer := blockedErrorWriter{started: make(chan struct{}), release: make(chan struct{})}
	defer close(writer.release)
	streams := newStreams(nil, writer, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	workDone := make(chan struct{})
	go func() {
		done <- streams.wait(ctx, nil, func() error {
			defer close(workDone)
			_, err := (streamWriter{streams: streams}).Write([]byte("output"))
			return err
		})
	}()
	select {
	case <-writer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not start")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("returned before the writer finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	writer.release <- struct{}{}
	select {
	case err := <-done:
		var pathErr *os.PathError
		if !errors.Is(err, context.Canceled) || !errors.Is(err, io.ErrClosedPipe) ||
			!errors.As(err, &pathErr) || pathErr.Path != "artifact" {
			t.Fatalf("missing cancellation or typed write error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not finish")
	}
	<-workDone
}

func TestWaitPreservesCancellationDuringFinalization(t *testing.T) {
	writer := blockedErrorWriter{started: make(chan struct{}), release: make(chan struct{})}
	defer close(writer.release)
	streams := newStreams(nil, writer, nil)
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		_, _ = (streamWriter{streams: streams}).Write([]byte("output"))
	}()
	<-writer.started

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workDone := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- streams.wait(ctx, nil, func() error {
			close(workDone)
			return net.ErrClosed
		})
	}()
	<-workDone
	select {
	case err := <-done:
		t.Fatalf("returned before output detached: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	writer.release <- struct{}{}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, net.ErrClosed) ||
			!errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("lost cancellation or operation errors: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not finish")
	}
	<-writeDone
}
