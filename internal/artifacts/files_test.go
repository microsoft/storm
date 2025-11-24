package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile_Success(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "copyfile-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a source file with some content
	srcPath := filepath.Join(tmpDir, "source.txt")
	testContent := []byte("Hello, World! This is a test file.")
	err = os.WriteFile(srcPath, testContent, 0o644)
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Copy the file
	destPath := filepath.Join(tmpDir, "destination.txt")
	bytesCopied, err := CopyFile(srcPath, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify bytes copied
	expectedBytes := int64(len(testContent))
	if bytesCopied != expectedBytes {
		t.Errorf("expected %d bytes copied, got %d", expectedBytes, bytesCopied)
	}

	// Verify destination file exists and has correct content
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if string(destContent) != string(testContent) {
		t.Errorf("content mismatch: expected '%s', got '%s'", string(testContent), string(destContent))
	}
}

func TestCopyFile_CreatesNestedDirectories(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "copyfile-nested-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	testContent := []byte("nested directory test")
	err = os.WriteFile(srcPath, testContent, 0o644)
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Copy to a destination with nested directories that don't exist
	destPath := filepath.Join(tmpDir, "level1", "level2", "level3", "destination.txt")
	bytesCopied, err := CopyFile(srcPath, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify bytes copied
	if bytesCopied != int64(len(testContent)) {
		t.Errorf("expected %d bytes copied, got %d", len(testContent), bytesCopied)
	}

	// Verify destination file exists
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if string(destContent) != string(testContent) {
		t.Errorf("content mismatch: expected '%s', got '%s'", string(testContent), string(destContent))
	}
}

func TestCopyFile_SourceDoesNotExist(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "copyfile-noexist-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Try to copy a non-existent file
	srcPath := filepath.Join(tmpDir, "nonexistent.txt")
	destPath := filepath.Join(tmpDir, "destination.txt")

	bytesCopied, err := CopyFile(srcPath, destPath)
	if err == nil {
		t.Fatal("expected error for non-existent source file, got nil")
	}

	if bytesCopied != 0 {
		t.Errorf("expected 0 bytes copied, got %d", bytesCopied)
	}

	// Verify destination was not created
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("destination file should not exist")
	}
}

func TestCopyFile_EmptyFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "copyfile-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an empty source file
	srcPath := filepath.Join(tmpDir, "empty.txt")
	err = os.WriteFile(srcPath, []byte{}, 0o644)
	if err != nil {
		t.Fatalf("failed to create empty source file: %v", err)
	}

	// Copy the empty file
	destPath := filepath.Join(tmpDir, "empty-dest.txt")
	bytesCopied, err := CopyFile(srcPath, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify 0 bytes copied
	if bytesCopied != 0 {
		t.Errorf("expected 0 bytes copied for empty file, got %d", bytesCopied)
	}

	// Verify destination exists and is empty
	destContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if len(destContent) != 0 {
		t.Errorf("expected empty destination file, got %d bytes", len(destContent))
	}
}

func TestCopyFile_LargeFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "copyfile-large-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a larger source file (1MB)
	srcPath := filepath.Join(tmpDir, "large.bin")
	testContent := make([]byte, 1024*1024) // 1MB
	for i := range testContent {
		testContent[i] = byte(i % 256)
	}
	err = os.WriteFile(srcPath, testContent, 0o644)
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Copy the file
	destPath := filepath.Join(tmpDir, "large-dest.bin")
	bytesCopied, err := CopyFile(srcPath, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify bytes copied
	if bytesCopied != int64(len(testContent)) {
		t.Errorf("expected %d bytes copied, got %d", len(testContent), bytesCopied)
	}

	// Verify file size matches
	destInfo, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("failed to stat destination file: %v", err)
	}

	if destInfo.Size() != int64(len(testContent)) {
		t.Errorf("destination file size mismatch: expected %d, got %d", len(testContent), destInfo.Size())
	}
}

func TestMkdirParents_Success(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mkdirparents-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested path
	targetPath := filepath.Join(tmpDir, "level1", "level2", "level3", "file.txt")
	err = MkdirParents(targetPath, 0o755)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify parent directories were created
	parentDir := filepath.Dir(targetPath)
	info, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("parent directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected parent path to be a directory")
	}
}

func TestMkdirParents_AlreadyExists(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mkdirparents-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a directory structure
	existingDir := filepath.Join(tmpDir, "existing", "path")
	err = os.MkdirAll(existingDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create existing dir: %v", err)
	}

	// Try to create parents for a path in the existing directory
	targetPath := filepath.Join(existingDir, "file.txt")
	err = MkdirParents(targetPath, 0o755)
	if err != nil {
		t.Fatalf("unexpected error when parents already exist: %v", err)
	}

	// Verify directory still exists
	info, err := os.Stat(existingDir)
	if err != nil {
		t.Fatalf("directory should still exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected path to be a directory")
	}
}

func TestMkdirParents_RootLevel(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mkdirparents-root-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create path with file directly in temp dir (no nested dirs to create)
	targetPath := filepath.Join(tmpDir, "file.txt")
	err = MkdirParents(targetPath, 0o755)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify temp dir still exists (it should, we're just ensuring parents exist)
	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("temp directory should exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected temp dir to be a directory")
	}
}

func TestMkdirParents_CustomPermissions(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mkdirparents-perms-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create path with custom permissions
	targetPath := filepath.Join(tmpDir, "custom", "perms", "file.txt")
	err = MkdirParents(targetPath, 0o700)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify parent directories were created
	parentDir := filepath.Join(tmpDir, "custom", "perms")
	info, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("parent directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected parent path to be a directory")
	}

	// Note: Actual permission checking can be platform-dependent and affected by umask,
	// so we just verify the directory was created
}
