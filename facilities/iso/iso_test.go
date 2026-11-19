package iso

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func requireTool(t *testing.T) {
	t.Helper()
	if _, err := findTool(); err != nil {
		t.Skip(err)
	}
}

func TestBuildWritesLabelAndFiles(t *testing.T) {
	requireTool(t)
	out := filepath.Join(t.TempDir(), "seed.iso")
	files := fstest.MapFS{
		"ignition/config.ign": {Data: []byte(`{"ignition":{"version":"3.3.0"}}`)},
		"Mixed_Case.txt":      {Data: []byte("hello")},
	}

	if err := Build(context.Background(), out, "SEED", files); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// The primary volume descriptor starts at sector 16; the volume id is at offset 40.
	pvd := raw[16*2048:]
	if got := string(bytes.TrimRight(pvd[40:72], " ")); got != "SEED" {
		t.Errorf("volume label = %q, want SEED", got)
	}
	if !bytes.Contains(raw, []byte(`{"ignition":{"version":"3.3.0"}}`)) {
		t.Error("file contents not found in image")
	}
}

func TestBuildRejectsBadLabel(t *testing.T) {
	for _, label := range []string{"", "has space", "toolong_toolong_toolong_toolong_toolong", "a/b"} {
		err := Build(context.Background(), filepath.Join(t.TempDir(), "x.iso"), label, fstest.MapFS{})
		if err == nil {
			t.Errorf("label %q accepted", label)
		}
	}
}

func TestBuildLeavesNoOutputOnFailure(t *testing.T) {
	requireTool(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "x.iso")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Build(ctx, out, "SEED", fstest.MapFS{"a": {Data: []byte("a")}}); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("leftover files after failure: %v", entries)
	}
}

func TestBuildRejectsSymlinks(t *testing.T) {
	requireTool(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(src, "file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "dir")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file", "dir"} {
		sub := t.TempDir()
		if err := os.Symlink(filepath.Join(src, name), filepath.Join(sub, name)); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "x.iso")
		if err := Build(context.Background(), out, "SEED", os.DirFS(sub)); err == nil {
			t.Errorf("symlink %q accepted", name)
		}
		if _, err := os.Stat(out); err == nil {
			t.Errorf("output left behind for %q", name)
		}
	}
}
