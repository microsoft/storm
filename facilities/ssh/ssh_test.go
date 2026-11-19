package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// server runs an in-process SSH server that accepts one key and answers a few
// fixed commands: "echo-ok" prints ok, "fail" exits 3, anything else exits 127.
func server(t *testing.T, authorized ssh.PublicKey) string {
	return serverWithHandler(t, authorized, serveChannel)
}

func serverWithHandler(t *testing.T, authorized ssh.PublicKey, handler func(*ssh.ServerConn, ssh.NewChannel)) string {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if string(k.Marshal()) == string(authorized.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hostSigner)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var connections sync.Map
	var workers sync.WaitGroup
	workers.Add(1)
	t.Cleanup(func() {
		l.Close()
		connections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		workers.Wait()
	})

	go func() {
		defer workers.Done()
		for {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			connections.Store(nc, struct{}{})
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer connections.Delete(nc)
				serveWithHandler(nc, cfg, handler)
			}()
		}
	}()
	return l.Addr().String()
}

func serve(nc net.Conn, cfg *ssh.ServerConfig) {
	serveWithHandler(nc, cfg, serveChannel)
}

func serveWithHandler(nc net.Conn, cfg *ssh.ServerConfig, handler func(*ssh.ServerConn, ssh.NewChannel)) {
	defer nc.Close()
	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		go handler(conn, nch)
	}
}

func serveChannel(conn *ssh.ServerConn, nch ssh.NewChannel) {
	if nch.ChannelType() != "session" {
		_ = nch.Reject(ssh.UnknownChannelType, "no")
		return
	}
	ch, creqs, err := nch.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	for r := range creqs {
		if (r.Type == "subsystem" && string(r.Payload[4:]) == "sftp") ||
			(r.Type == "exec" && string(r.Payload[4:]) == "sudo -n /test/sftp-server") {
			_ = r.Reply(true, nil)
			srv, err := sftp.NewServer(ch)
			if err == nil {
				_ = srv.Serve()
			}
			return
		}
		if r.Type != "exec" {
			_ = r.Reply(false, nil)
			continue
		}
		cmd := string(r.Payload[4:])
		_ = r.Reply(true, nil)
		status := byte(127)
		switch cmd {
		case "echo-ok":
			_, _ = ch.Write([]byte("ok\n"))
			status = 0
		case "fail":
			_, _ = ch.Stderr().Write([]byte("bad\n"))
			status = 3
		case "copy-input":
			_, _ = io.Copy(ch, ch)
			status = 0
		case "large-output":
			var writes sync.WaitGroup
			writes.Add(2)
			go func() {
				defer writes.Done()
				_, _ = ch.Write(bytes.Repeat([]byte("o"), 3<<20))
			}()
			go func() {
				defer writes.Done()
				_, _ = ch.Stderr().Write(bytes.Repeat([]byte("e"), 3<<20))
			}()
			writes.Wait()
			status = 0
		case "missing-status":
			return
		case "disconnect":
			_ = conn.Close()
			return
		case "hang":
			time.Sleep(3 * time.Second)
			return
		case "signal":
			_, _ = ch.SendRequest("exit-signal", false,
				ssh.Marshal(struct {
					Signal     string
					CoreDumped bool
					Error      string
					Lang       string
				}{"KILL", false, "killed", ""}))
			return
		}
		_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, status})
		return
	}
}

func TestGenerateKeyIsUsable(t *testing.T) {
	k, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.AuthorizedKey, "ssh-ed25519 ") || !strings.HasSuffix(k.AuthorizedKey, "\n") {
		t.Errorf("unexpected authorized key %q", k.AuthorizedKey)
	}
	parsed, err := ssh.ParsePrivateKey(k.PEM)
	if err != nil {
		t.Fatalf("PEM does not parse: %v", err)
	}
	if string(parsed.PublicKey().Marshal()) != string(k.Signer.PublicKey().Marshal()) {
		t.Error("PEM and Signer are different keys")
	}
}

func TestRunReportsOutputAndExitCode(t *testing.T) {
	key, _ := GenerateKey()
	addr := server(t, key.Signer.PublicKey())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Dial(ctx, Config{Addr: addr, User: "core", Auth: key.Signer, HostKey: InsecureHostKey()})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := c.Run(ctx, "echo-ok")
	if err != nil || res.ExitCode != 0 || string(res.Stdout) != "ok\n" {
		t.Errorf("echo-ok: res=%+v err=%v", res, err)
	}
	res, err = c.Run(ctx, "fail")
	if err != nil || res.ExitCode != 3 || string(res.Stderr) != "bad\n" {
		t.Errorf("fail: res=%+v err=%v", res, err)
	}
}

func TestDialRejectsWrongKeyAndMissingHostKey(t *testing.T) {
	good, _ := GenerateKey()
	other, _ := GenerateKey()
	addr := server(t, good.Signer.PublicKey())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := Dial(ctx, Config{Addr: addr, User: "core", Auth: other.Signer, HostKey: InsecureHostKey()}); err == nil {
		t.Error("wrong key accepted")
	}
	if _, err := Dial(ctx, Config{Addr: addr, User: "core", Auth: good.Signer}); err == nil {
		t.Error("missing HostKey accepted")
	}
}

func TestWaitReadyRetriesUntilServerAppears(t *testing.T) {
	key, _ := GenerateKey()
	// Reserve a port, release it, and bring the server up on it after a delay.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		time.Sleep(300 * time.Millisecond)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		defer l.Close()
		stop := context.AfterFunc(ctx, func() { l.Close() })
		defer stop()
		_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
		hs, _ := ssh.NewSignerFromKey(hostPriv)
		cfg := &ssh.ServerConfig{NoClientAuth: true}
		cfg.AddHostKey(hs)
		for ctx.Err() == nil {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			go serve(nc, cfg)
		}
	}()

	cfg := Config{Addr: addr, User: "core", Auth: key.Signer, HostKey: InsecureHostKey(), DialTimeout: time.Second}
	c, err := WaitReady(ctx, cfg, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	cancel()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not stop the listener")
	}
}

func TestWaitReadyStopsWhenContextEnds(t *testing.T) {
	key, _ := GenerateKey()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cfg := Config{Addr: addr, User: "core", Auth: key.Signer, HostKey: InsecureHostKey(), DialTimeout: 100 * time.Millisecond}
	if _, err := WaitReady(ctx, cfg, 50*time.Millisecond); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchCopiesRemoteFile(t *testing.T) {
	key, _ := GenerateKey()
	addr := server(t, key.Signer.PublicKey())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	path := filepath.Join(t.TempDir(), "remote.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Dial(ctx, Config{Addr: addr, User: "core", Auth: key.Signer, HostKey: InsecureHostKey()})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var buf bytes.Buffer
	if err := c.Fetch(ctx, path, &buf); err != nil || buf.String() != "payload" {
		t.Errorf("fetch: %q, %v", buf.String(), err)
	}
	if err := c.Fetch(ctx, path+".missing", &buf); err == nil {
		t.Error("missing file did not fail")
	}
}

func dialCfg(addr string, key *Key) Config {
	return Config{Addr: addr, User: "core", Auth: key.Signer, HostKey: InsecureHostKey(), DialTimeout: time.Second}
}

func TestRunSignalHasNoExitCode(t *testing.T) {
	key, _ := GenerateKey()
	addr := server(t, key.Signer.PublicKey())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, dialCfg(addr, key))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := c.Run(ctx, "signal")
	if err != nil || res.ExitCode != -1 || res.Signal != "KILL" {
		t.Errorf("res=%+v err=%v", res, err)
	}
}

func TestRunCancelOnlyAffectsThatCommand(t *testing.T) {
	key, _ := GenerateKey()
	addr := server(t, key.Signer.PublicKey())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Dial(ctx, dialCfg(addr, key))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	short, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	defer stop()
	res, err := c.Run(short, "hang")
	if !errors.Is(err, context.DeadlineExceeded) || res.ExitCode != -1 {
		t.Errorf("hang: res=%+v err=%v", res, err)
	}
	res, err = c.Run(ctx, "echo-ok")
	if err != nil || res.ExitCode != 0 {
		t.Errorf("client unusable after a cancelled command: res=%+v err=%v", res, err)
	}
}

func TestDialCancelDuringHandshake(t *testing.T) {
	// A listener that accepts and never speaks SSH.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			defer nc.Close()
		}
	}()
	key, _ := GenerateKey()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cfg := dialCfg(l.Addr().String(), key)
	cfg.DialTimeout = 30 * time.Second

	start := time.Now()
	_, err := Dial(ctx, cfg)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Errorf("err=%v after %v", err, time.Since(start))
	}
}

func TestWaitReadyFailsFastOnPermanentErrors(t *testing.T) {
	good, _ := GenerateKey()
	other, _ := GenerateKey()
	addr := server(t, good.Signer.PublicKey())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	check := func(name string, cfg Config) {
		t.Helper()
		start := time.Now()
		_, err := WaitReady(ctx, cfg, time.Second)
		if err == nil || time.Since(start) > 900*time.Millisecond {
			t.Errorf("%s: err=%v after %v", name, err, time.Since(start))
		}
	}
	check("wrong key", dialCfg(addr, other))
	cfg := dialCfg(addr, good)
	cfg.HostKey = nil
	check("no host key callback", cfg)
	cfg = dialCfg(addr, good)
	cfg.HostKey = ssh.FixedHostKey(good.Signer.PublicKey())
	check("host key mismatch", cfg)
}

func TestWaitReadyRetriesAuthFailureWhenAsked(t *testing.T) {
	key, _ := GenerateKey()
	other, _ := GenerateKey()
	// The server accepts "other" first, then switches to key.
	var accept atomic.Value
	accept.Store(other.Signer.PublicKey())
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hostPriv)
	scfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if string(k.Marshal()) == string(accept.Load().(ssh.PublicKey).Marshal()) {
			return &ssh.Permissions{}, nil
		}
		return nil, ssh.ErrNoAuth
	}}
	scfg.AddHostKey(hs)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			go serve(nc, scfg)
		}
	}()
	time.AfterFunc(300*time.Millisecond, func() { accept.Store(key.Signer.PublicKey()) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := dialCfg(l.Addr().String(), key)
	cfg.RetryAuthFailure = true
	c, err := WaitReady(ctx, cfg, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}
