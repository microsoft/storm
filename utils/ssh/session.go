package ssh

import (
	"context"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

func (c *Client) session(ctx context.Context) (*ssh.Session, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	type result struct {
		session *ssh.Session
		err     error
	}
	ready := make(chan result)
	go func() {
		session, err := c.conn.NewSession()
		select {
		case ready <- result{session, err}:
		case <-ctx.Done():
			if session != nil {
				_ = session.Close()
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case result := <-ready:
		if result.err != nil {
			return nil, nil, result.err
		}
		// Channel-open and request acknowledgments cannot be interrupted by
		// x/crypto/ssh without closing the shared connection. Close late
		// sessions when the peer responds, without blocking cancellation.
		closeSession := sync.OnceFunc(func() { go result.session.Close() })
		stop := context.AfterFunc(ctx, func() {
			go result.session.Signal(ssh.SIGKILL)
			closeSession()
		})
		return result.session, func() {
			stop()
			closeSession()
		}, nil
	}
}

type streams struct {
	inputMu  sync.Mutex
	outputMu sync.Mutex
	input    io.Reader
	output   io.Writer
	errOut   io.Writer
	failed   chan struct{}
	errOnce  sync.Once
	err      error
}

func newStreams(input io.Reader, output, errOut io.Writer) *streams {
	return &streams{input: input, output: output, errOut: errOut, failed: make(chan struct{})}
}

func (s *streams) fail(err error) {
	s.errOnce.Do(func() {
		s.err = err
		close(s.failed)
	})
}

func (s *streams) failure() error {
	select {
	case <-s.failed:
		return s.err
	default:
		return nil
	}
}

func (s *streams) collectCleanup(err *error, cleanup func() error) {
	// Record the existing error before cleanup can block.
	if *err != nil {
		s.fail(*err)
	}
	*err = errors.Join(*err, cleanup())
}

func (s *streams) Read(p []byte) (int, error) {
	s.inputMu.Lock()
	input := s.input
	s.inputMu.Unlock()
	if input == nil {
		return 0, io.EOF
	}
	n, err := input.Read(p)
	if err != nil && err != io.EOF {
		s.fail(err)
	}
	return n, err
}

func (s *streams) detach() {
	s.outputMu.Lock()
	s.output, s.errOut = nil, nil
	s.outputMu.Unlock()
	s.inputMu.Lock()
	s.input = nil
	s.inputMu.Unlock()
}

func (s *streams) wait(ctx context.Context, interrupt <-chan struct{}, work func() error) (err error) {
	defer func() {
		s.detach()
		for _, cause := range []error{s.failure(), ctx.Err()} {
			if cause != nil && !errors.Is(err, cause) {
				err = errors.Join(err, cause)
			}
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		if err := ctx.Err(); err != nil {
			done <- err
			return
		}
		done <- work()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-interrupt:
		return s.failure()
	case err := <-done:
		return err
	}
}

type streamWriter struct {
	streams *streams
	stderr  bool
}

func (w streamWriter) Write(p []byte) (int, error) {
	s := w.streams
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	dst := s.output
	if w.stderr {
		dst = s.errOut
	}
	if dst == nil {
		return len(p), nil
	}
	n, err := dst.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		s.fail(err)
	}
	return n, err
}
