package iso

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestStageWindowsPathsStayLocal(t *testing.T) {
	for _, name := range []string{`..\escaped`, `nested\..\..\escaped`, `C:\escaped`, `NUL`} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			staging := filepath.Join(root, "staging")
			if err := os.Mkdir(staging, 0o700); err != nil {
				t.Fatal(err)
			}
			files := fstest.MapFS{name: {Data: []byte("must stay local")}}
			if err := stage(context.Background(), staging, files); err == nil {
				t.Fatal("accepted a non-local Windows path")
			}
			if _, err := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(err) {
				t.Fatalf("wrote outside staging: %v", err)
			}
		})
	}
}
