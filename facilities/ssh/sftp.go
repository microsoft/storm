package ssh

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
)

var sudoSFTPPaths = [...]string{
	"/usr/libexec/sftp-server",
	"/usr/lib/openssh/sftp-server",
	"/usr/libexec/openssh/sftp-server",
	"/usr/lib/ssh/sftp-server",
	"/usr/local/libexec/sftp-server",
}

// SFTP is a scoped sudo file-transfer handle returned by Client.SudoSFTP.
// It borrows its client's connection; each transfer owns its SFTP session, so
// the handle itself needs no Close call.
type SFTP struct {
	client     *Client
	serverPath string
}

// SudoSFTP selects passwordless, non-interactive sudo for this handle only.
// With no argument, the first transfer discovers a standalone server in standard
// OpenSSH locations and caches the successful path per SSH client. PATH is not
// searched. Pass one absolute executable path, not a command, to bypass discovery.
//
// Construction performs no remote I/O. Invalid paths or multiple arguments return
// an error. Client defaults are unchanged; failures never fall back to ordinary
// transfers. Hosts with only internal-sftp need a standalone server installed.
func (c *Client) SudoSFTP(serverPath ...string) (*SFTP, error) {
	if len(serverPath) > 1 {
		return nil, fmt.Errorf("SudoSFTP accepts at most one server path")
	}
	files := &SFTP{client: c}
	if len(serverPath) == 1 {
		if !path.IsAbs(serverPath[0]) || strings.ContainsRune(serverPath[0], '\x00') {
			return nil, fmt.Errorf("SFTP server path must be absolute and contain no NUL bytes")
		}
		files.serverPath = serverPath[0]
	}
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("SudoSFTP requires an SSH client")
	}
	return files, nil
}

// Download downloads through sudo using the same ownership, cancellation and error
// semantics as Client.Download. Discovery runs within ctx and is not cached on error.
func (s *SFTP) Download(ctx context.Context, remotePath string, dst io.Writer) error {
	if dst == nil {
		return fmt.Errorf("download destination is nil")
	}
	command, err := s.command(ctx)
	if err != nil {
		return err
	}
	if err := s.client.download(ctx, remotePath, dst, command); err != nil {
		return fmt.Errorf("sudo SFTP download (passwordless sudo required): %w", err)
	}
	return nil
}

// Upload sends src through sudo with Client.Upload's private staging and
// replacement semantics. The uploaded file is owned by sudo's target user.
// Discovery runs within ctx; src remains caller-owned.
func (s *SFTP) Upload(ctx context.Context, remotePath string, src io.Reader) error {
	if remotePath == "" {
		return fmt.Errorf("upload path is empty")
	}
	if src == nil {
		return fmt.Errorf("upload source is nil")
	}
	command, err := s.command(ctx)
	if err != nil {
		return err
	}
	if err := s.client.upload(ctx, remotePath, src, command); err != nil {
		return fmt.Errorf("sudo SFTP upload (passwordless sudo required): %w", err)
	}
	return nil
}

func (s *SFTP) command(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	server := s.serverPath
	if server == "" {
		var err error
		server, err = s.client.discoverSFTP(ctx)
		if err != nil {
			return "", err
		}
	}
	return "sudo -n -- " + shellQuote(server), nil
}

func (c *Client) discoverSFTP(ctx context.Context) (string, error) {
	c.sudoMu.Lock()
	cached := c.sudoPath
	c.sudoMu.Unlock()
	if cached != "" {
		return cached, nil
	}
	for _, candidate := range sudoSFTPPaths {
		quoted := shellQuote(candidate)
		result, err := c.RunIO(ctx, "test -f "+quoted+" && test -x "+quoted, nil, nil, nil)
		if err != nil {
			return "", fmt.Errorf("discover sudo SFTP server: %w", err)
		}
		switch result.ExitCode {
		case 0:
			c.sudoMu.Lock()
			if c.sudoPath == "" {
				c.sudoPath = candidate
			}
			cached = c.sudoPath
			c.sudoMu.Unlock()
			return cached, nil
		case 1:
			continue
		default:
			return "", fmt.Errorf("SFTP discovery probe exited %d (signal %q)", result.ExitCode, result.Signal)
		}
	}
	return "", fmt.Errorf("no standalone SFTP server found in standard locations; provide an explicit server path (internal-sftp cannot run under sudo)")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
