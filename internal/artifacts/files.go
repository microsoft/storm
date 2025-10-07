package artifacts

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies a file from srcPath to destPath.
// It returns the number of bytes copied and any error encountered.
func CopyFile(srcPath, destPath string) (int64, error) {
	input, err := os.Open(srcPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open source file %s: %w", srcPath, err)
	}
	defer input.Close()

	err = os.MkdirAll(filepath.Dir(destPath), 0o755)
	if err != nil {
		return 0, fmt.Errorf("failed to create directory for destination file %s: %w", destPath, err)
	}

	output, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination file %s: %w", destPath, err)
	}
	defer output.Close()

	bytesCopied, err := io.Copy(output, input)
	if err != nil {
		return bytesCopied, fmt.Errorf("failed to copy data from %s to %s: %w", srcPath, destPath, err)
	}

	return bytesCopied, nil
}

// MkdirParents creates all parent directories for the given path with the specified permissions.
func MkdirParents(path string, perm os.FileMode) error {
	err := os.MkdirAll(filepath.Dir(path), perm)
	if err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %w", path, err)
	}

	return nil
}
