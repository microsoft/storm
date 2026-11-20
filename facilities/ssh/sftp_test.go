package ssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type sudoFixture struct {
	probes       map[string]bool
	servers      map[string]bool
	probeStarted chan struct{}
	probeRelease chan struct{}
	probeOnce    sync.Once
	mu           sync.Mutex
	commands     []string
}

func standardSudoFixture(serverPath string) *sudoFixture {
	return &sudoFixture{
		probes:  map[string]bool{"test -f '" + serverPath + "' && test -x '" + serverPath + "'": true},
		servers: map[string]bool{"sudo -n -- '" + serverPath + "'": true},
	}
}

func (f *sudoFixture) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *sudoFixture) serve(_ *ssh.ServerConn, pending ssh.NewChannel) {
	ch, requests, err := pending.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	request, ok := <-requests
	if !ok {
		return
	}
	if request.Type == "subsystem" && string(request.Payload[4:]) == "sftp" {
		f.mu.Lock()
		f.commands = append(f.commands, "subsystem:sftp")
		f.mu.Unlock()
		_ = request.Reply(true, nil)
		serveSFTPFixture(ch)
		return
	}
	if request.Type != "exec" {
		_ = request.Reply(false, nil)
		return
	}
	var payload struct{ Command string }
	if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
		_ = request.Reply(false, nil)
		return
	}
	command := payload.Command
	f.mu.Lock()
	f.commands = append(f.commands, command)
	f.mu.Unlock()
	_ = request.Reply(true, nil)
	status := uint32(0)
	switch {
	case strings.HasPrefix(command, "test -f "):
		if f.probeRelease != nil {
			f.probeOnce.Do(func() { close(f.probeStarted) })
			<-f.probeRelease
		}
		if !f.probes[command] {
			status = 1
		}
	case strings.HasPrefix(command, "sudo -n -- "):
		if f.servers[command] {
			serveSFTPFixture(ch)
			return
		}
		_, _ = io.WriteString(ch.Stderr(), "sudo: a password is required\n")
		status = 1
	case command == "base-server":
		serveSFTPFixture(ch)
		return
	case command == "echo-ok":
		_, _ = io.WriteString(ch, "ok\n")
	default:
		status = 127
	}
	_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
}

func serveSFTPFixture(ch ssh.Channel) {
	server, err := sftp.NewServer(ch)
	if err != nil {
		return
	}
	defer server.Close()
	_ = server.Serve()
}

func sudoTestClient(t *testing.T, fixture *sudoFixture, baseCommand string) (*Client, context.Context) {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := dialCfg(serverWithHandler(t, key.Signer.PublicKey(), fixture.serve), key)
	cfg.SFTPCommand = baseCommand
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client, ctx
}

func countCommands(commands []string, prefix string) int {
	count := 0
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) {
			count++
		}
	}
	return count
}

func TestSudoSFTPDiscoversCachesAndKeepsClientDefaults(t *testing.T) {
	fixture := standardSudoFixture("/usr/libexec/openssh/sftp-server")
	fixture.servers["sudo -n -- '/opt/custom/sftp-server'"] = true
	client, ctx := sudoTestClient(t, fixture, "base-server")
	files, err := client.SudoSFTP()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.snapshot()) != 0 {
		t.Fatal("construction performed remote I/O")
	}
	remote := filepath.Join(t.TempDir(), "config")
	if err := files.Upload(ctx, remote, strings.NewReader("private")); err != nil {
		t.Fatal(err)
	}
	probes := countCommands(fixture.snapshot(), "test -f ")
	if probes != 3 {
		t.Fatalf("got %d discovery probes, want 3", probes)
	}
	another, err := client.SudoSFTP()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := another.Download(ctx, remote, &output); err != nil || output.String() != "private" {
		t.Fatalf("sudo download: %q %v", output.String(), err)
	}
	if countCommands(fixture.snapshot(), "test -f ") != probes {
		t.Fatal("a second handle did not reuse the discovered path")
	}
	override, err := client.SudoSFTP("/opt/custom/sftp-server")
	if err != nil {
		t.Fatal(err)
	}
	if err := override.Download(ctx, remote, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := another.Download(ctx, remote, io.Discard); err != nil {
		t.Fatal(err)
	}
	commands := fixture.snapshot()
	if commands[len(commands)-1] != "sudo -n -- '/usr/libexec/openssh/sftp-server'" {
		t.Fatal("explicit override replaced the auto-discovery cache")
	}
	if err := client.Download(ctx, remote, io.Discard); err != nil {
		t.Fatal(err)
	}
	if countCommands(fixture.snapshot(), "base-server") != 1 {
		t.Fatal("sudo handle changed the client's configured transfers")
	}
	info, err := os.Stat(remote)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private upload mode: %v %v", info, err)
	}
}

func TestSudoSFTPExplicitPathSkipsDiscoveryAndQuotes(t *testing.T) {
	command := "sudo -n -- '/opt/server'\"'\"'s sftp;$(id)'"
	fixture := &sudoFixture{servers: map[string]bool{command: true}}
	client, ctx := sudoTestClient(t, fixture, "")
	files, err := client.SudoSFTP("/opt/server's sftp;$(id)")
	if err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(t.TempDir(), "config")
	if err := files.Upload(ctx, remote, strings.NewReader("value")); err != nil {
		t.Fatal(err)
	}
	if err := files.Download(ctx, remote, io.Discard); err != nil {
		t.Fatal(err)
	}
	commands := fixture.snapshot()
	if len(commands) != 2 || commands[0] != command || commands[1] != command {
		t.Fatalf("manual path was probed or quoted incorrectly: %q", commands)
	}
	if err := client.Download(ctx, remote, io.Discard); err != nil {
		t.Fatal(err)
	}
	if countCommands(fixture.snapshot(), "subsystem:sftp") != 1 {
		t.Fatal("ordinary transfers no longer use the default subsystem")
	}
}

func TestSudoSFTPRejectsInvalidPathsWithoutIO(t *testing.T) {
	fixture := &sudoFixture{}
	client, _ := sudoTestClient(t, fixture, "")
	for _, paths := range [][]string{{""}, {"internal-sftp"}, {"relative/server"}, {"/bad\x00path"}, {"/one", "/two"}} {
		if _, err := client.SudoSFTP(paths...); err == nil {
			t.Errorf("accepted invalid paths: %q", paths)
		}
	}
	if len(fixture.snapshot()) != 0 {
		t.Fatal("invalid configuration performed remote I/O")
	}
}

func TestSudoSFTPDoesNotDowngradeOnFailure(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "sudo denied"
		fixture := standardSudoFixture("/usr/libexec/sftp-server")
		fixture.servers = nil
		if missing {
			name, fixture.probes = "no standalone server", nil
		}
		t.Run(name, func(t *testing.T) {
			client, ctx := sudoTestClient(t, fixture, "")
			files, err := client.SudoSFTP()
			if err != nil {
				t.Fatal(err)
			}
			remote := filepath.Join(t.TempDir(), "config")
			err = files.Upload(ctx, remote, strings.NewReader("must not be written"))
			if err == nil || (!strings.Contains(err.Error(), "passwordless sudo") && !strings.Contains(err.Error(), "explicit server path")) {
				t.Fatalf("missing actionable sudo failure: %v", err)
			}
			if _, err := os.Stat(remote); !os.IsNotExist(err) {
				t.Fatalf("failed sudo transfer created a file: %v", err)
			}
			if countCommands(fixture.snapshot(), "subsystem:sftp") != 0 {
				t.Fatal("privileged transfer fell back to the ordinary subsystem")
			}
		})
	}
}

func TestSudoSFTPDiscoveryCancellationDoesNotPoisonCache(t *testing.T) {
	fixture := standardSudoFixture("/usr/libexec/sftp-server")
	fixture.probeStarted, fixture.probeRelease = make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(fixture.probeRelease) })
	defer release()
	client, ctx := sudoTestClient(t, fixture, "")
	files, err := client.SudoSFTP()
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- files.Download(short, "/missing", io.Discard) }()
	select {
	case <-fixture.probeStarted:
	case <-ctx.Done():
		t.Fatal("discovery did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled discovery did not return")
	}
	release()
	remote := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(remote, []byte("value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := files.Download(ctx, remote, io.Discard); err != nil {
		t.Fatalf("cancelled discovery poisoned the next transfer: %v", err)
	}
}
