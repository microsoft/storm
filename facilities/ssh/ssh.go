// Package ssh is a small SSH client for talking to freshly booted test machines.
//
// It covers ephemeral key generation, waiting for a machine to accept
// connections, running a command, and fetching a file. Commands are never
// retried: only connection establishment is.
package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Key is an in-memory key pair. Nothing is written to disk; callers decide
// whether to persist PEM (with mode 0600).
type Key struct {
	Signer ssh.Signer
	// AuthorizedKey is the public key in authorized_keys format, newline-terminated.
	AuthorizedKey string
	// PEM is the OpenSSH-format private key.
	PEM []byte
}

// GenerateKey creates a new ed25519 key pair.
func GenerateKey() (*Key, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("wrap key: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("encode public key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, fmt.Errorf("encode private key: %w", err)
	}
	return &Key{
		Signer:        signer,
		AuthorizedKey: string(ssh.MarshalAuthorizedKey(sshPub)),
		PEM:           pem.EncodeToMemory(block),
	}, nil
}

// InsecureHostKey accepts any host key. It is only appropriate for machines
// created and destroyed by the same test run, whose host key cannot be known
// in advance. Callers must pass it explicitly.
func InsecureHostKey() ssh.HostKeyCallback { return ssh.InsecureIgnoreHostKey() } //nolint:gosec

// Config describes how to connect.
type Config struct {
	// Addr is host:port.
	Addr string
	User string
	Auth ssh.Signer
	// HostKey is required.
	HostKey ssh.HostKeyCallback
	// DialTimeout bounds one connection attempt. Defaults to 10 seconds.
	DialTimeout time.Duration
	// RetryAuthFailure makes WaitReady keep retrying when the server rejects
	// the key. Off by default so a wrong key fails fast. Turn it on for a
	// machine that may start accepting connections before it has installed the
	// authorized key.
	RetryAuthFailure bool
}

var (
	errConfig  = errors.New("invalid configuration")
	errHostKey = errors.New("host key rejected")
	errAuth    = errors.New("authentication failed")
)

func (c Config) client() (*ssh.ClientConfig, error) {
	if c.HostKey == nil {
		return nil, fmt.Errorf("ssh: %w: HostKey is required (use InsecureHostKey for disposable machines)", errConfig)
	}
	if c.Auth == nil {
		return nil, fmt.Errorf("ssh: %w: Auth is required", errConfig)
	}
	timeout := c.DialTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &ssh.ClientConfig{
		User:            c.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(c.Auth)},
		HostKeyCallback: c.HostKey,
		Timeout:         timeout,
	}, nil
}

// Client is a connected SSH client.
type Client struct {
	conn *ssh.Client
}

// Dial connects once. ctx covers the TCP connection and the SSH handshake.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	cc, err := cfg.client()
	if err != nil {
		return nil, err
	}
	var hostKeyErr error
	check := cc.HostKeyCallback
	cc.HostKeyCallback = func(host string, remote net.Addr, key ssh.PublicKey) error {
		hostKeyErr = check(host, remote, key)
		return hostKeyErr
	}

	nc, err := (&net.Dialer{Timeout: cc.Timeout}).DialContext(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", cfg.Addr, err)
	}
	// Closing the connection is what interrupts a handshake in progress.
	stop := context.AfterFunc(ctx, func() { nc.Close() })

	// The handshake has its own deadline: the dialer's timeout ended with the dial.
	_ = nc.SetDeadline(time.Now().Add(cc.Timeout))
	sc, chans, reqs, err := ssh.NewClientConn(nc, cfg.Addr, cc)
	if err == nil {
		_ = nc.SetDeadline(time.Time{})
	}
	if !stop() || err != nil {
		nc.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("handshake with %s: %w", cfg.Addr, ctxErr)
		}
		switch {
		case hostKeyErr != nil:
			err = fmt.Errorf("%w: %w", errHostKey, err)
		case strings.Contains(err.Error(), "unable to authenticate"):
			err = fmt.Errorf("%w: %w", errAuth, err)
		}
		return nil, fmt.Errorf("handshake with %s: %w", cfg.Addr, err)
	}
	return &Client{conn: ssh.NewClient(sc, chans, reqs)}, nil
}

// WaitReady retries Dial every interval until it succeeds or ctx ends. Errors
// that waiting cannot fix (invalid configuration, a rejected host key and,
// unless Config.RetryAuthFailure is set, a rejected key) are returned at once.
// No connection is left open on failure.
func WaitReady(ctx context.Context, cfg Config, interval time.Duration) (*Client, error) {
	var last error
	for {
		c, err := Dial(ctx, cfg)
		if err == nil {
			return c, nil
		}
		if errors.Is(err, errConfig) || errors.Is(err, errHostKey) ||
			(errors.Is(err, errAuth) && !cfg.RetryAuthFailure) {
			return nil, err
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for ssh on %s: %w (last attempt: %v)", cfg.Addr, ctx.Err(), last)
		case <-time.After(interval):
		}
	}
}

// Result is the outcome of a command.
type Result struct {
	Stdout []byte
	Stderr []byte
	// ExitCode is the remote exit status, or -1 when there is none: the
	// command was killed by a signal, the connection was lost, or ctx ended.
	ExitCode int
	// Signal is the name of the signal that terminated the command, if any.
	Signal string
}

// Run executes command and waits for it. A non-zero exit status is reported in
// Result.ExitCode with a nil error; an error means the command could not be
// run or its outcome is unknown, and Result.ExitCode is then -1.
//
// Cancelling ctx closes only this command's session, so other commands on the
// same Client are unaffected. The remote process is signalled on a best-effort
// basis; many servers ignore that, and it may keep running.
func (c *Client) Run(ctx context.Context, command string) (Result, error) {
	failed := Result{ExitCode: -1}
	s, err := c.conn.NewSession()
	if err != nil {
		return failed, fmt.Errorf("open session: %w", err)
	}
	defer s.Close()

	var stdout, stderr bytes.Buffer
	s.Stdout, s.Stderr = &stdout, &stderr

	done := make(chan error, 1)
	go func() { done <- s.Run(command) }()

	var runErr error
	select {
	case <-ctx.Done():
		_ = s.Signal(ssh.SIGKILL)
		s.Close()
		<-done // buffers are safe to read once Run returns
		runErr = ctx.Err()
	case runErr = <-done:
	}

	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exit *ssh.ExitError
	switch {
	case runErr == nil:
	case ctx.Err() != nil && errors.Is(runErr, ctx.Err()):
		res.ExitCode = -1
		return res, fmt.Errorf("run %q: %w", command, runErr)
	case errors.As(runErr, &exit):
		res.ExitCode = exit.ExitStatus()
		if sig := exit.Signal(); sig != "" {
			res.ExitCode, res.Signal = -1, sig
		}
	default:
		res.ExitCode = -1
		return res, fmt.Errorf("run %q: %w", command, runErr)
	}
	return res, nil
}

// Fetch copies a remote file to dst over SFTP. Cancelling ctx interrupts a
// transfer in progress.
func (c *Client) Fetch(ctx context.Context, remotePath string, dst io.Writer) error {
	sc, err := sftp.NewClient(c.conn)
	if err != nil {
		return fmt.Errorf("start sftp: %w", err)
	}
	defer sc.Close()
	// Closing the sftp client fails any request blocked on the wire.
	defer context.AfterFunc(ctx, func() { sc.Close() })()

	f, err := sc.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", remotePath, err)
	}
	defer f.Close()

	if _, err := io.Copy(dst, f); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return fmt.Errorf("read %s: %w", remotePath, err)
	}
	return nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }
