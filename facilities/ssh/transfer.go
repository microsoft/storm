package ssh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/pkg/sftp"
)

// Fetch copies a remote file to dst. Config.SFTPCommand selects a custom
// server, such as a passwordless sudo command. dst remains caller-owned.
// Cancellation covers startup and reads without closing other sessions.
// File-open, copy, file-close and SFTP-close errors are returned.
func (c *Client) Fetch(ctx context.Context, remotePath string, dst io.Writer) error {
	return c.fetch(ctx, remotePath, dst, c.sftpCommand)
}

func (c *Client) fetch(ctx context.Context, remotePath string, dst io.Writer, command string) error {
	if dst == nil {
		return fmt.Errorf("fetch destination is nil")
	}
	streams := newStreams(nil, dst, nil)
	return c.transfer(ctx, streams, command, func(sc *sftp.Client) (err error) {
		file, err := sc.Open(remotePath)
		if err != nil {
			return fmt.Errorf("open %s: %w", remotePath, err)
		}
		defer func() { err = errors.Join(err, file.Close()) }()
		_, err = io.Copy(streamWriter{streams: streams}, file)
		return err
	})
}

// Upload writes src to a remote file owned by the SFTP server user with mode
// 0600, staging in a private directory before renaming. Atomic replacement
// requires the server's POSIX rename extension; without it, an existing
// destination causes an error. Replacement does not follow destination symlinks
// or preserve existing ownership/mode. An error after rename or a lost
// acknowledgment does not imply the destination is unchanged.
// Files and transfer sessions are closed, with close errors returned. src
// remains caller-owned; callers must unblock pending reads on cancellation.
// Abrupt disconnections can leave a private staging directory on the server.
func (c *Client) Upload(ctx context.Context, remotePath string, src io.Reader) error {
	return c.upload(ctx, remotePath, src, c.sftpCommand)
}

func (c *Client) upload(ctx context.Context, remotePath string, src io.Reader, command string) error {
	if remotePath == "" {
		return fmt.Errorf("upload path is empty")
	}
	if src == nil {
		return fmt.Errorf("upload source is nil")
	}
	streams := newStreams(src, nil, nil)
	return c.transfer(ctx, streams, command, func(sc *sftp.Client) (err error) {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		dir := path.Join(path.Dir(remotePath), ".storm-upload-"+hex.EncodeToString(random[:]))
		if err := sc.Mkdir(dir); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, sc.RemoveDirectory(dir)) }()
		if err := sc.Chmod(dir, 0o700); err != nil {
			return err
		}
		temporary := path.Join(dir, "upload")
		file, err := sc.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return err
		}
		defer func() {
			if removeErr := sc.Remove(temporary); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, removeErr)
			}
		}()
		writeErr := file.Chmod(0o600)
		if writeErr == nil {
			_, writeErr = io.Copy(file, streams)
		}
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, supported := sc.HasExtension("posix-rename@openssh.com"); supported {
			return sc.PosixRename(temporary, remotePath)
		}
		return sc.Rename(temporary, remotePath)
	})
}

func (c *Client) transfer(ctx context.Context, streams *streams, command string, work func(*sftp.Client) error) error {
	session, closeSession, err := c.session(ctx)
	if err != nil {
		return fmt.Errorf("open transfer session: %w", err)
	}
	defer closeSession()
	err = streams.wait(ctx, nil, func() (err error) {
		stdin, err := session.StdinPipe()
		if err != nil {
			return err
		}
		stdout, err := session.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := session.StderrPipe()
		if err != nil {
			return err
		}
		go io.Copy(io.Discard, stderr)
		if command == "" {
			err = session.RequestSubsystem("sftp")
		} else {
			err = session.Start(command)
		}
		if err != nil {
			return fmt.Errorf("start sftp: %w", err)
		}
		sc, err := sftp.NewClientPipe(stdout, stdin)
		if err != nil {
			return fmt.Errorf("negotiate sftp: %w", err)
		}
		defer func() { err = errors.Join(err, sc.Close()) }()
		if err := ctx.Err(); err != nil {
			return err
		}
		return work(sc)
	})
	if err != nil {
		return fmt.Errorf("sftp transfer: %w", err)
	}
	return nil
}
