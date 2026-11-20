package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

func testClient(t *testing.T, command string) (*Client, context.Context) {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := dialCfg(server(t, key.Signer.PublicKey()), key)
	cfg.SFTPCommand = command
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client, ctx
}

func TestRunIOStreamsInputAndConcurrentOutput(t *testing.T) {
	client, ctx := testClient(t, "")
	var stdout, stderr bytes.Buffer
	result, err := client.RunIO(ctx, "copy-input", strings.NewReader("input"), &stdout, &stderr)
	if err != nil || result.ExitCode != 0 || stdout.String() != "input" || stderr.Len() != 0 {
		t.Fatalf("stdin: result=%+v stdout=%q stderr=%q err=%v", result, stdout.String(), stderr.String(), err)
	}
	stdout.Reset()
	result, err = client.RunIO(ctx, "large-output", nil, &stdout, &stderr)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("large output: result=%+v err=%v", result, err)
	}
	if !bytes.Equal(stdout.Bytes(), bytes.Repeat([]byte("o"), 3<<20)) ||
		!bytes.Equal(stderr.Bytes(), bytes.Repeat([]byte("e"), 3<<20)) {
		t.Fatal("large stdout/stderr were not fully delivered")
	}
	if result.Stdout != nil || result.Stderr != nil {
		t.Fatal("RunIO retained streamed output")
	}

	var combined bytes.Buffer
	if _, err := client.RunIO(ctx, "large-output", nil, &combined, &combined); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(combined.Bytes(), []byte("o")) != 3<<20 || bytes.Count(combined.Bytes(), []byte("e")) != 3<<20 {
		t.Fatal("combined writer lost output")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestRunIOPropagatesWriterErrors(t *testing.T) {
	sentinel := errors.New("writer failed")
	for _, tt := range []struct {
		name   string
		writer io.Writer
		stderr bool
		want   error
	}{
		{"stdout", failingWriter{sentinel}, false, sentinel},
		{"stderr", failingWriter{sentinel}, true, sentinel},
		{"short write", shortWriter{}, false, io.ErrShortWrite},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, ctx := testClient(t, "")
			var stdout, stderr io.Writer = io.Discard, io.Discard
			if tt.stderr {
				stderr = tt.writer
			} else {
				stdout = tt.writer
			}
			result, err := client.RunIO(ctx, "large-output", nil, stdout, stderr)
			if !errors.Is(err, tt.want) || result.ExitCode != -1 {
				t.Fatalf("result=%+v err=%v, want %v", result, err, tt.want)
			}
			if result, err := client.Run(ctx, "echo-ok"); err != nil || result.ExitCode != 0 {
				t.Fatalf("writer failure affected the connection: %+v %v", result, err)
			}
		})
	}
}

func TestRunIOPropagatesInputError(t *testing.T) {
	client, ctx := testClient(t, "")
	sentinel := errors.New("stdin failed")
	input := readerFunc(func([]byte) (int, error) { return 0, sentinel })
	result, err := client.RunIO(ctx, "copy-input", input, io.Discard, nil)
	if !errors.Is(err, sentinel) || result.ExitCode != -1 {
		t.Fatalf("result=%+v err=%v, want %v", result, err, sentinel)
	}
}

func TestCommandResultErrorsRemainTyped(t *testing.T) {
	client, ctx := testClient(t, "")
	var stderr bytes.Buffer
	result, err := client.RunIO(ctx, "fail", nil, nil, &stderr)
	if err != nil || result.ExitCode != 3 || stderr.String() != "bad\n" {
		t.Fatalf("nonzero exit: result=%+v stderr=%q err=%v", result, stderr.String(), err)
	}
	result, err = client.Run(ctx, "missing-status")
	var missing *ssh.ExitMissingError
	if result.ExitCode != -1 || !errors.As(err, &missing) {
		t.Fatalf("missing status: result=%+v err=%v", result, err)
	}
}

func TestDialContextDoesNotOwnConnectedClient(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := dialCfg(server(t, key.Signer.PublicKey()), key)
	dialCtx, stopDial := context.WithCancel(context.Background())
	defer stopDial()
	client, err := Dial(dialCtx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	stopDial()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if result, err := client.Run(ctx, "echo-ok"); err != nil || result.ExitCode != 0 {
		t.Fatalf("dial context closed the client: %+v %v", result, err)
	}
	if err := client.WaitDisconnected(dialCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait on live connection: %v", err)
	}
	result, err := client.Run(ctx, "disconnect")
	var missing *ssh.ExitMissingError
	if result.ExitCode != -1 || !errors.As(err, &missing) {
		t.Fatalf("disconnect: result=%+v err=%v", result, err)
	}
	if err := client.WaitDisconnected(ctx); err != nil {
		t.Fatalf("remote disconnect not observed: %v", err)
	}
}

func TestStartupCancellationKeepsConnectionUsable(t *testing.T) {
	for _, operation := range []string{"run", "fetch", "upload", "fetch-command", "upload-command"} {
		for _, stage := range []string{"channel", "request", "version"} {
			if operation == "run" && stage == "version" {
				continue
			}
			t.Run(operation+"/"+stage, func(t *testing.T) {
				key, err := GenerateKey()
				if err != nil {
					t.Fatal(err)
				}
				started, release := make(chan struct{}), make(chan struct{})
				defer close(release)
				var channels atomic.Int32
				addr := serverWithHandler(t, key.Signer.PublicKey(), func(conn *ssh.ServerConn, pending ssh.NewChannel) {
					if channels.Add(1) != 1 {
						serveChannel(conn, pending)
						return
					}
					if stage == "channel" {
						close(started)
						<-release
						_ = pending.Reject(ssh.Prohibited, "cancelled")
						return
					}
					ch, requests, err := pending.Accept()
					if err != nil {
						return
					}
					defer ch.Close()
					request, ok := <-requests
					if !ok {
						return
					}
					if stage == "version" {
						_ = request.Reply(true, nil)
						if _, err := io.ReadFull(ch, make([]byte, 9)); err != nil {
							return
						}
					}
					close(started)
					<-release
				})
				cfg := dialCfg(addr, key)
				if strings.HasSuffix(operation, "-command") {
					cfg.SFTPCommand = "sudo -n /test/sftp-server"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				client, err := Dial(ctx, cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				short, stop := context.WithCancel(ctx)
				defer stop()
				done := make(chan error, 1)
				remote := filepath.Join(t.TempDir(), "target")
				go func() {
					switch {
					case operation == "run":
						result, err := client.Run(short, "echo-ok")
						if result.ExitCode != -1 {
							err = fmt.Errorf("unexpected exit code %d: %w", result.ExitCode, err)
						}
						done <- err
					case strings.HasPrefix(operation, "fetch"):
						done <- client.Fetch(short, remote, io.Discard)
					default:
						done <- client.Upload(short, remote, strings.NewReader("input"))
					}
				}()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("server did not reach the negotiation stage")
				}
				stop()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("wanted cancellation, got %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("cancellation waited for a server acknowledgment")
				}
				result, err := client.Run(ctx, "echo-ok")
				if err != nil || result.ExitCode != 0 {
					t.Fatalf("cancelled startup affected another session: %+v %v", result, err)
				}
			})
		}
	}
}

func TestUploadAndFetchWithDefaultAndCustomServer(t *testing.T) {
	for _, command := range []string{"", "sudo -n /test/sftp-server"} {
		t.Run(command, func(t *testing.T) {
			client, ctx := testClient(t, command)
			dir := t.TempDir()
			remote := filepath.Join(dir, "config")
			for _, content := range []string{"first input", "replacement"} {
				input := strings.NewReader(content)
				source := readerFunc(func(p []byte) (int, error) {
					files, err := filepath.Glob(filepath.Join(dir, ".storm-upload-*", "upload"))
					if err != nil || len(files) != 1 {
						return 0, fmt.Errorf("private staging file missing: %v %v", files, err)
					}
					file, err := os.Stat(files[0])
					if err != nil || file.Mode().Perm() != 0o600 {
						return 0, fmt.Errorf("file is not private before reading input: %v %v", file, err)
					}
					parent, err := os.Stat(filepath.Dir(files[0]))
					if err != nil || parent.Mode().Perm() != 0o700 {
						return 0, fmt.Errorf("staging directory is not private: %v %v", parent, err)
					}
					return input.Read(p)
				})
				if err := client.Upload(ctx, remote, source); err != nil {
					t.Fatal(err)
				}
				var output bytes.Buffer
				if err := client.Fetch(ctx, remote, &output); err != nil {
					t.Fatal(err)
				}
				if output.String() != content {
					t.Errorf("got %q, want %q", output.String(), content)
				}
				info, err := os.Stat(remote)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("uploaded file mode: %v %v", info, err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("upload left staging files: %v %v", entries, err)
			}
			sentinel := errors.New("destination writer failed")
			if err := client.Fetch(ctx, remote, failingWriter{sentinel}); !errors.Is(err, sentinel) {
				t.Fatalf("lost writer error: %v", err)
			}
			sentinel = errors.New("source reader failed")
			if err := client.Upload(ctx, remote, readerFunc(func([]byte) (int, error) { return 0, sentinel })); !errors.Is(err, sentinel) {
				t.Fatalf("lost source reader error: %v", err)
			}
			content, err := os.ReadFile(remote)
			if err != nil || string(content) != "replacement" {
				t.Fatalf("failed upload replaced the destination: %q %v", content, err)
			}
			entries, err = os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed upload left staging files: %v %v", entries, err)
			}
		})
	}
}

type stalledFileReader struct {
	inner      sftp.FileReader
	started    chan struct{}
	release    chan struct{}
	once       sync.Once
	stallClose bool
}

func (r *stalledFileReader) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	reader, err := r.inner.Fileread(request)
	if err != nil {
		return nil, err
	}
	return &stalledReaderAt{ReaderAt: reader, control: r}, nil
}

type stalledReaderAt struct {
	io.ReaderAt
	control *stalledFileReader
}

func (r *stalledReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if !r.control.stallClose {
		r.control.wait()
	}
	return r.ReaderAt.ReadAt(p, offset)
}

func (r *stalledReaderAt) Close() error {
	if r.control.stallClose {
		r.control.wait()
	}
	if closer, ok := r.ReaderAt.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (r *stalledFileReader) wait() {
	r.once.Do(func() { close(r.started) })
	<-r.release
}

func TestFetchCancellationDuringRead(t *testing.T) {
	for _, stage := range []string{"read", "close"} {
		t.Run(stage, func(t *testing.T) {
			key, err := GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			handlers := sftp.InMemHandler()
			blocked := &stalledFileReader{
				inner: handlers.FileGet, started: make(chan struct{}), release: make(chan struct{}),
				stallClose: stage == "close",
			}
			defer close(blocked.release)
			handlers.FileGet = blocked
			var channels atomic.Int32
			addr := serverWithHandler(t, key.Signer.PublicKey(), func(conn *ssh.ServerConn, pending ssh.NewChannel) {
				if channels.Add(1) > 2 {
					serveChannel(conn, pending)
					return
				}
				ch, requests, err := pending.Accept()
				if err != nil {
					return
				}
				defer ch.Close()
				request, ok := <-requests
				if !ok {
					return
				}
				_ = request.Reply(true, nil)
				server := sftp.NewRequestServer(ch, handlers)
				defer server.Close()
				_ = server.Serve()
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client, err := Dial(ctx, dialCfg(addr, key))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			seed, err := sftp.NewClient(client.conn)
			if err != nil {
				t.Fatal(err)
			}
			file, err := seed.Create("/source")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write([]byte("payload")); err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(file.Close(), seed.Close()); err != nil {
				t.Fatal(err)
			}
			short, stop := context.WithCancel(ctx)
			defer stop()
			var dst io.Writer = io.Discard
			if blocked.stallClose {
				dst = failingWriter{&os.PathError{Op: "write", Path: "artifact", Err: io.ErrClosedPipe}}
			}
			done := make(chan error, 1)
			go func() { done <- client.Fetch(short, "/source", dst) }()
			select {
			case <-blocked.started:
			case <-ctx.Done():
				t.Fatalf("transfer did not reach file %s", stage)
			}
			stop()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("wanted cancellation, got %v", err)
				}
				if blocked.stallClose {
					var pathErr *os.PathError
					if !errors.Is(err, io.ErrClosedPipe) || !errors.As(err, &pathErr) || pathErr.Path != "artifact" {
						t.Fatalf("lost the original writer error during cancellation: %v", err)
					}
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("cancelled file %s did not return", stage)
			}
			if result, err := client.Run(ctx, "echo-ok"); err != nil || result.ExitCode != 0 {
				t.Fatalf("cancelled transfer affected another session: %+v %v", result, err)
			}
		})
	}
}
