package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscardCloser_NoOp(t *testing.T) {
	p := []byte("hello")
	n, err := discarder.Write(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(p) {
		t.Fatalf("expected %d bytes written, got %d", len(p), n)
	}

	if err := discarder.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestOpenStreamManager_NewStream_WriteClose(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "artifact.txt")

	m := &openStreamManager{}

	s, err := m.newStream(dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := []byte("streamed-data")
	if _, err := s.Write(payload); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("content mismatch: expected %q, got %q", string(payload), string(content))
	}

	if m.streams != nil {
		if _, ok := m.streams[dest]; ok {
			t.Fatalf("expected stream to be removed from manager after close")
		}
	}
}

func TestOpenStreamManager_DuplicateDestinationBlockedUntilClose(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "artifact.txt")

	m := &openStreamManager{}

	s1, err := m.newStream(dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := m.newStream(dest); err == nil {
		t.Fatalf("expected duplicate open to error")
	}

	if err := s1.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	// After close, should be allowed again.
	s2, err := m.newStream(dest)
	if err != nil {
		t.Fatalf("expected open after close to succeed, got error: %v", err)
	}
	_ = s2.Close()
}

func TestOpenStreamManager_CloseStream_NoPanicWhenMissing(t *testing.T) {
	m := &openStreamManager{}
	m.closeStream("/does/not/exist")
}
