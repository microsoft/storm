// Package ssh is a small SSH client for talking to freshly booted test machines.
//
// It covers ephemeral key generation, waiting for a machine to accept
// connections, running commands, and transferring files. Commands are never
// retried: only connection establishment is.
package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Key is an in-memory key pair. Nothing is written to disk; callers decide
// whether to persist PEM (with mode 0600).
type Key struct {
	// Signer authenticates connections through Config.Auth.
	Signer ssh.Signer
	// AuthorizedKey is the public key in authorized_keys format, newline-terminated.
	AuthorizedKey string
	// PEM is the OpenSSH-format private key.
	PEM []byte
}

// GenerateKey creates an in-memory ed25519 key pair. Install AuthorizedKey in
// the guest before connecting; check that its crypto policy permits ed25519.
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
	// User is the remote login name.
	User string
	// Auth is the required authentication key; other signer types are accepted.
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
	// SFTPCommand starts a caller-selected SFTP server instead of the default
	// subsystem, for example a passwordless sudo command. No command is retried.
	SFTPCommand string
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

// Client is a connected SSH client returned by Dial or WaitReady. Call Close
// when finished; operation cancellation does not close the shared connection.
type Client struct {
	conn         *ssh.Client
	sftpCommand  string
	disconnected chan struct{}
}

// Dial connects once. ctx covers the TCP connection and the SSH handshake.
// The returned client remains usable after ctx ends and must be closed by
// the caller. Config.HostKey must explicitly verify or accept the host key.
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
	client := &Client{
		conn:         ssh.NewClient(sc, chans, reqs),
		sftpCommand:  cfg.SFTPCommand,
		disconnected: make(chan struct{}),
	}
	go func() {
		_ = client.conn.Wait()
		close(client.disconnected)
	}()
	return client, nil
}

// WaitReady retries Dial every interval until it succeeds or ctx ends. Errors
// that waiting cannot fix (invalid configuration, a rejected host key and,
// unless Config.RetryAuthFailure is set, a rejected key) are returned at once.
// No connection is left open on failure.
// Use a bounded ctx; the returned client's lifetime is independent of it.
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

// WaitDisconnected returns nil when the connection closes, locally or remotely,
// or ctx.Err() if waiting is cancelled. It does not probe or reconnect; a silent
// network failure may remain undetected until ctx expires.
func (c *Client) WaitDisconnected(ctx context.Context) error {
	select {
	case <-c.disconnected:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close closes the connection and all its sessions, releasing pending startup
// operations. It returns the underlying connection's close error.
func (c *Client) Close() error { return c.conn.Close() }
