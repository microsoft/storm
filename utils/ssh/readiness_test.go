package ssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestWaitReadyProbesExecutionAndClosesFailedConnections(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int32
	closed := make(chan struct{}, 2)
	addr := serverWithHandler(t, key.Signer.PublicKey(), func(conn *ssh.ServerConn, pending ssh.NewChannel) {
		ch, requests, err := pending.Accept()
		if err != nil {
			return
		}
		defer ch.Close()
		request, ok := <-requests
		if !ok {
			return
		}
		var payload struct{ Command string }
		if request.Type != "exec" || ssh.Unmarshal(request.Payload, &payload) != nil {
			_ = request.Reply(false, nil)
			return
		}
		_ = request.Reply(true, nil)
		status := uint32(0)
		switch payload.Command {
		case "true":
			if probes.Add(1) < 3 {
				status = 1
				go func() {
					_ = conn.Wait()
					closed <- struct{}{}
				}()
			}
		case "echo-ok":
			_, _ = io.WriteString(ch, "ok\n")
		default:
			status = 127
		}
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := WaitReady(ctx, dialCfg(addr, key), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if probes.Load() != 3 {
		t.Fatalf("got %d probes, want 3", probes.Load())
	}
	for range 2 {
		select {
		case <-closed:
		case <-ctx.Done():
			t.Fatal("a failed probe left its connection open")
		}
	}
	cancel()
	operationCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	result, err := client.Run(operationCtx, "echo-ok")
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "ok\n" {
		t.Fatalf("returned client depended on readiness context: %+v %v", result, err)
	}
}

func TestWaitReadySkipExecCheckSupportsSFTPOnlyHosts(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	var executions atomic.Int32
	rejected := make(chan struct{}, 1)
	addr := serverWithHandler(t, key.Signer.PublicKey(), func(_ *ssh.ServerConn, pending ssh.NewChannel) {
		ch, requests, err := pending.Accept()
		if err != nil {
			return
		}
		defer ch.Close()
		request, ok := <-requests
		if !ok {
			return
		}
		if request.Type != "subsystem" || string(request.Payload[4:]) != "sftp" {
			executions.Add(1)
			_ = request.Reply(false, nil)
			select {
			case rejected <- struct{}{}:
			default:
			}
			return
		}
		_ = request.Reply(true, nil)
		server, err := sftp.NewServer(ch)
		if err != nil {
			return
		}
		defer server.Close()
		_ = server.Serve()
	})
	cfg := dialCfg(addr, key)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	direct, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	direct.Close()
	if executions.Load() != 0 {
		t.Fatal("Dial performed an execution check")
	}

	cfg.SkipExecCheck = true
	client, err := WaitReady(ctx, cfg, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if executions.Load() != 0 {
		t.Fatal("SkipExecCheck still attempted command execution")
	}
	remote := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(remote, []byte("SFTP only"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := client.Download(ctx, remote, &output); err != nil || output.String() != "SFTP only" {
		t.Fatalf("SFTP-only download: %q %v", output.String(), err)
	}

	cfg.SkipExecCheck = false
	probeCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() {
		unready, err := WaitReady(probeCtx, cfg, time.Millisecond)
		if unready != nil {
			unready.Close()
		}
		done <- err
	}()
	select {
	case <-rejected:
	case <-ctx.Done():
		t.Fatal("default execution check was not attempted")
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || executions.Load() == 0 {
			t.Fatalf("default probe did not reject exec-disabled host: probes=%d err=%v", executions.Load(), err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("default execution check did not stop")
	}
}

func TestWaitReadyCancelsStalledExecutionProbe(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	closed := make(chan struct{})
	addr := serverWithHandler(t, key.Signer.PublicKey(), func(conn *ssh.ServerConn, pending ssh.NewChannel) {
		ch, requests, err := pending.Accept()
		if err != nil {
			return
		}
		defer ch.Close()
		if _, ok := <-requests; !ok {
			return
		}
		go func() {
			_ = conn.Wait()
			close(closed)
		}()
		close(started)
		<-release
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		client, err := WaitReady(ctx, dialCfg(addr, key), time.Second)
		if client != nil {
			client.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("execution probe did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stalled probe ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled probe left its connection open")
	}
}
