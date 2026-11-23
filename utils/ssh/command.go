package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// Result is the outcome of a command.
type Result struct {
	// Stdout is captured by Run and nil for RunIO.
	Stdout []byte
	// Stderr is captured separately by Run and nil for RunIO.
	Stderr []byte
	// ExitCode is the remote exit status, or -1 if unavailable.
	ExitCode int
	// Signal is the remote termination signal, if any.
	Signal string
}

// Run buffers command output. Nonzero remote exit statuses are returned in
// Result with a nil error. Transport and I/O errors remain wrapped for errors.As.
// Unavailable exit statuses are -1. Cancellation follows RunIO's session-only
// semantics; commands are never retried.
func (c *Client) Run(ctx context.Context, command string) (Result, error) {
	var stdout, stderr bytes.Buffer
	result, err := c.RunIO(ctx, command, nil, &stdout, &stderr)
	result.Stdout, result.Stderr = stdout.Bytes(), stderr.Bytes()
	return result, err
}

// RunIO streams command I/O without retaining output in Result. Nil streams
// mean EOF/discard. stdout and stderr are consumed concurrently; writes are
// serialized, so both may use the same writer. Commands are never retried.
//
// Cancellation stops waiting during startup and execution without closing the
// shared connection. Pending negotiations are released when the peer responds
// or Client.Close is called. Remote process termination is not guaranteed.
// Caller streams are not closed; writers must not block indefinitely, and
// callers must unblock pending reads from stdin themselves when cancelling.
func (c *Client) RunIO(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) (Result, error) {
	result := Result{ExitCode: -1}
	session, closeSession, err := c.session(ctx)
	if err != nil {
		return result, fmt.Errorf("open session: %w", err)
	}
	defer closeSession()
	streams := newStreams(stdin, stdout, stderr)
	if stdin != nil {
		session.Stdin = streams
	}
	session.Stdout = streamWriter{streams: streams}
	session.Stderr = streamWriter{streams: streams, stderr: true}

	err = streams.wait(ctx, streams.failed, func() error { return session.Run(command) })
	ioErr, ctxErr := streams.failure(), ctx.Err()
	if ioErr != nil || ctxErr != nil {
		for _, cause := range []error{ioErr, ctxErr} {
			if cause != nil && !errors.Is(err, cause) {
				err = errors.Join(err, cause)
			}
		}
		return result, fmt.Errorf("run %q: %w", command, err)
	}
	var exit *ssh.ExitError
	switch {
	case err == nil:
		result.ExitCode = 0
	case errors.As(err, &exit):
		result.ExitCode = exit.ExitStatus()
		if sig := exit.Signal(); sig != "" {
			result.ExitCode, result.Signal = -1, sig
		}
	default:
		return result, fmt.Errorf("run %q: %w", command, err)
	}
	return result, nil
}
