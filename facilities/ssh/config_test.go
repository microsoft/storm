package ssh

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestWaitReadyRejectsInvalidConfiguration(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		addr    string
		timeout time.Duration
	}{
		{addr: "127.0.0.1"},
		{addr: "127.0.0.1:"},
		{addr: "[::1"},
		{addr: "127.0.0.1:65536"},
		{addr: "127.0.0.1:no-such-service"},
		{addr: "127.0.0.1:22", timeout: -time.Second},
	} {
		t.Run(tt.addr+"/"+tt.timeout.String(), func(t *testing.T) {
			cfg := dialCfg(tt.addr, key)
			cfg.DialTimeout = tt.timeout
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := WaitReady(ctx, cfg, time.Hour); !errors.Is(err, errConfig) {
				t.Fatalf("want a configuration error without retries, got %v", err)
			}
		})
	}
	// Named TCP services remain valid, just as with net.Dialer.
	if _, err := dialCfg("localhost:ssh", key).client(); err != nil {
		t.Fatalf("valid service name rejected: %v", err)
	}
}

type deadlineConn struct {
	net.Conn
	deadline time.Time
}

func (c *deadlineConn) SetDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		c.deadline = deadline
	}
	return c.Conn.SetDeadline(deadline)
}

func TestDialSharesOneAttemptDeadline(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	local, remote := net.Pipe()
	conn := &deadlineConn{Conn: local}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, remote)
	}()
	defer func() {
		local.Close()
		remote.Close()
		<-done
	}()

	cfg := dialCfg("127.0.0.1:22", key)
	cfg.DialTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var attemptDeadline time.Time
	connect := func(ctx context.Context, _, _ string) (net.Conn, error) {
		attemptDeadline, _ = ctx.Deadline()
		time.Sleep(10 * time.Millisecond)
		return conn, nil
	}
	if _, err := dial(ctx, cfg, connect); err == nil {
		t.Fatal("expected the stalled handshake to time out")
	}
	if attemptDeadline.IsZero() || !conn.deadline.Equal(attemptDeadline) {
		t.Fatalf("handshake deadline %s differs from connection deadline %s", conn.deadline, attemptDeadline)
	}
}
