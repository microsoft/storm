package ssh

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type uploadHandlers struct {
	sftp.FileWriter
	sftp.FileCmder
	reject atomic.Bool
}

func (h *uploadHandlers) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	writer, err := h.FileWriter.Filewrite(request)
	if err != nil {
		return nil, err
	}
	return &uploadWriterAt{WriterAt: writer, handlers: h}, nil
}

func (h *uploadHandlers) Filecmd(request *sftp.Request) error {
	if request.Method == "Setstat" {
		return nil // The in-memory backend does not implement directory chmod.
	}
	return h.FileCmder.Filecmd(request)
}

func (h *uploadHandlers) PosixRename(request *sftp.Request) error {
	return h.FileCmder.(sftp.PosixRenameFileCmder).PosixRename(request)
}

type uploadWriterAt struct {
	io.WriterAt
	handlers *uploadHandlers
}

func (w *uploadWriterAt) WriteAt(p []byte, offset int64) (int, error) {
	if w.handlers.reject.Load() {
		return 0, os.ErrPermission
	}
	return w.WriterAt.WriteAt(p, offset)
}

func (w *uploadWriterAt) Close() error {
	if closer, ok := w.WriterAt.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type noSFTPExtensions struct {
	ssh.Channel
	sentVersion bool
}

func (c *noSFTPExtensions) Write(p []byte) (int, error) {
	if c.sentVersion {
		return c.Channel.Write(p)
	}
	c.sentVersion = true
	if len(p) < 9 || p[4] != 2 {
		return 0, fmt.Errorf("expected an SFTP version packet")
	}
	// Strip the initial VERSION extensions without changing server-wide defaults.
	version := append([]byte(nil), p[:9]...)
	binary.BigEndian.PutUint32(version[:4], 5)
	n, err := c.Channel.Write(version)
	if err == nil && n != len(version) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func TestUploadFailurePreservesDestination(t *testing.T) {
	for _, failure := range []string{"final write", "source truncation"} {
		t.Run(failure, func(t *testing.T) {
			handlers := sftp.InMemHandler()
			controlled := &uploadHandlers{FileWriter: handlers.FilePut, FileCmder: handlers.FileCmd}
			handlers.FilePut, handlers.FileCmd = controlled, controlled
			key, err := GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
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
			file, err := seed.Create("/config")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write([]byte("original")); err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(file.Close(), seed.Close()); err != nil {
				t.Fatal(err)
			}

			var source io.Reader = strings.NewReader("replacement")
			if failure == "final write" {
				controlled.reject.Store(true)
			} else {
				source = readerFunc(func(p []byte) (int, error) {
					return copy(p, "partial"), io.ErrUnexpectedEOF
				})
			}
			if err := client.Upload(ctx, "/config", source); err == nil {
				t.Error("upload reported success after failure")
			}
			var got bytes.Buffer
			if err := client.Download(ctx, "/config", &got); err != nil {
				t.Fatal(err)
			}
			if got.String() != "original" {
				t.Fatalf("failed upload replaced the destination: %q", got.String())
			}
		})
	}
}

func TestUploadWithoutPosixRename(t *testing.T) {
	handlers := sftp.InMemHandler()
	controlled := &uploadHandlers{FileWriter: handlers.FilePut, FileCmder: handlers.FileCmd}
	handlers.FilePut, handlers.FileCmd = controlled, controlled
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
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
		_ = request.Reply(true, nil)
		server := sftp.NewRequestServer(&noSFTPExtensions{Channel: ch}, handlers)
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
	inspection, err := sftp.NewClient(client.conn)
	if err != nil {
		t.Fatal(err)
	}
	defer inspection.Close()
	if _, supported := inspection.HasExtension("posix-rename@openssh.com"); supported {
		t.Fatal("test server advertised POSIX rename")
	}

	if err := client.Upload(ctx, "/config", strings.NewReader("original")); err != nil {
		t.Fatalf("ordinary rename to a new destination: %v", err)
	}
	if err := client.Upload(ctx, "/config", strings.NewReader("replacement")); err == nil {
		t.Fatal("ordinary rename overwrote an existing destination")
	}
	var got bytes.Buffer
	if err := client.Download(ctx, "/config", &got); err != nil {
		t.Fatal(err)
	}
	if got.String() != "original" {
		t.Fatalf("failed upload replaced the destination: %q", got.String())
	}
	entries, err := inspection.ReadDir("/")
	if err != nil || len(entries) != 1 || entries[0].Name() != "config" {
		t.Fatalf("uploads left staging files: %v %v", entries, err)
	}
}
