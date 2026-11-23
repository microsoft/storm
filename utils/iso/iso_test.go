package iso

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
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
	if err := os.WriteFile(out, []byte("old image"), 0o600); err != nil {
		t.Fatal(err)
	}
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

func TestBuildPreservesOutputOnAuthoringFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	toolDir := t.TempDir()
	tool := "xorrisofs"
	if runtime.GOOS == "windows" {
		tool += ".exe"
	}
	// The test executable rejects the authoring flags and exits unsuccessfully.
	if err := os.WriteFile(filepath.Join(toolDir, tool), binary, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolDir)
	dir := t.TempDir()
	out := filepath.Join(dir, "seed.iso")
	original := []byte("original image")
	if err := os.WriteFile(out, original, 0o600); err != nil {
		t.Fatal(err)
	}
	err = Build(context.Background(), out, "SEED", fstest.MapFS{"config": {Data: []byte("new")}})
	var failure *exec.ExitError
	if !errors.As(err, &failure) {
		t.Fatalf("expected an authoring-process failure, got %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("failed build changed the image: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "seed.iso" {
		t.Fatalf("failed build left temporary output: %v %v", entries, err)
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

func TestStageLocalPaths(t *testing.T) {
	dir := t.TempDir()
	files := fstest.MapFS{"nested/config.json": {Data: []byte("config")}}
	if runtime.GOOS != "windows" {
		files[`back\slash`] = &fstest.MapFile{Data: []byte("local")}
		files["name:with:colons"] = &fstest.MapFile{Data: []byte("local")}
	}
	if err := stage(context.Background(), dir, files); err != nil {
		t.Fatal(err)
	}
	for name, file := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(data, file.Data) {
			t.Errorf("%s: got %q, err=%v", name, data, err)
		}
	}
}

func TestStageRejectsNonDirectoryRoot(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode fs.FileMode
	}{
		{"file", 0},
		{"symlink", fs.ModeSymlink},
		{"pipe", fs.ModeNamedPipe},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files, err := fs.Sub(fstest.MapFS{"root": {Mode: tt.mode}}, "root")
			if err != nil {
				t.Fatal(err)
			}
			if err := stage(context.Background(), t.TempDir(), files); err == nil {
				t.Fatal("non-directory filesystem root accepted")
			}
		})
	}
}

func TestStageInsideSource(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "config"), []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(src, "staging")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := stage(ctx, dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil || string(got) != "seed" {
		t.Fatalf("staged config: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "config" {
		t.Fatalf("staging copied itself: %v %v", entries, err)
	}
}
