// Package iso builds small data ISOs (for example configuration or seed
// volumes that a guest finds by volume label).
//
// It is intentionally not a bootable-media authoring tool.
package iso

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ISO 9660 volume identifiers are at most 32 characters. The accepted set is
// deliberately conservative: it is what every mkisofs flavour handles alike.
var labelPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// mkisofs tools that can author an image from a directory, in order of preference.
var tools = []string{"xorrisofs", "genisoimage", "mkisofs"}

// Build writes an ISO image with the given volume label to output. The image
// holds the contents of files. Joliet and Rock Ridge extensions are enabled so
// that long, mixed-case names survive.
// It requires xorrisofs, genisoimage or mkisofs in PATH. label must contain
// 1-32 ASCII letters, digits, underscores or dashes.
//
// files must hold only directories and regular files; symbolic links and
// special files are rejected. files is trusted input that is not modified while
// Build runs: path checks alone cannot make a concurrently changing directory a
// safe boundary. ctx is checked between entries and cancels image creation.
// File names must be representable as local paths on the host.
//
// The output directory must already exist. Existing output is replaced only
// after a successful build; failed builds leave no partial output.
func Build(ctx context.Context, output, label string, files fs.FS) error {
	if !labelPattern.MatchString(label) {
		return fmt.Errorf("invalid volume label %q: want 1-32 characters from [A-Za-z0-9_-]", label)
	}

	tool, err := findTool()
	if err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "storm-iso-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := stage(ctx, staging, files); err != nil {
		return err
	}

	// Write beside the destination so the final rename cannot cross filesystems.
	tmp, err := os.CreateTemp(filepath.Dir(output), ".storm-iso-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	cmd := exec.CommandContext(ctx, tool,
		"-quiet", "-J", "-rational-rock", "-input-charset", "utf8",
		"-V", label, "-o", tmpName, staging)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w: %s", tool, err, strings.TrimSpace(string(out)))
	}

	if err := os.Rename(tmpName, output); err != nil {
		return fmt.Errorf("move image into place: %w", err)
	}
	return nil
}

func findTool() (string, error) {
	for _, t := range tools {
		if p, err := exec.LookPath(t); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("none of %s found in PATH", strings.Join(tools, ", "))
}

// stage copies files into dir, rejecting names that could escape it.
func stage(ctx context.Context, dir string, files fs.FS) error {
	return fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		local, err := filepath.Localize(name)
		if err != nil {
			return fmt.Errorf("invalid path %q: %w", name, err)
		}
		dst := filepath.Join(dir, local)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%q is a %s, not a regular file or directory", name, d.Type().Type())
		}
		return copyFile(files, name, dst)
	})
}

func copyFile(files fs.FS, name, dst string) (err error) {
	src, err := files.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()

	_, err = io.Copy(out, src)
	return err
}
