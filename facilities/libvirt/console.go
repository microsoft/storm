// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.

package libvirt

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

const (
	captureBackoffMin = 100 * time.Millisecond
	captureBackoffMax = 2 * time.Second
	// captureStable is how long a console stream must have lived for the
	// reopen backoff to reset.
	captureStable = 5 * time.Second
	// captureDrainWait bounds how long ForceStop and Shutdown wait for the
	// console stream to end after the domain has stopped.
	captureDrainWait = 5 * time.Second
)

// capture is one in-process serial console capture, keyed by domain name.
type capture struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (cp *capture) finished() bool {
	select {
	case <-cp.done:
		return true
	default:
		return false
	}
}

// captures tracks the serial console captures of a client. They are tied to
// the libvirt connection: shutdown cancels them for good.
type captures struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	m      map[string]*capture
	wg     sync.WaitGroup
}

func (cs *captures) init() {
	cs.ctx, cs.cancel = context.WithCancel(context.Background())
	cs.m = map[string]*capture{}
}

func (cs *captures) shutdown() {
	cs.mu.Lock()
	cs.cancel()
	cs.mu.Unlock()
}

// ctxWriter makes the console stream stop at the next chunk of output once
// ctx ends.
type ctxWriter struct {
	ctx context.Context
	w   io.Writer
}

func (c ctxWriter) Write(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.w.Write(p)
}

// startCapture begins appending the domain's serial console to path, a file
// on this machine created with the caller's own ownership. It is a no-op when
// path is empty or a capture for the domain is already running.
//
// The capture goroutine reopens the console stream if it ends while the domain
// is still alive, and exits once the domain is shut off or gone, the client is
// closed, or the capture is cancelled.
func (c *Client) startCapture(name, path string) error {
	if path == "" {
		return nil
	}
	cs := &c.caps
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cp := cs.m[name]; cp != nil && !cp.finished() {
		return nil
	}
	if cs.ctx.Err() != nil {
		return fmt.Errorf("client is closed")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open serial log: %w", err)
	}

	ctx, cancel := context.WithCancel(cs.ctx)
	cp := &capture{cancel: cancel, done: make(chan struct{})}
	cs.m[name] = cp
	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		defer close(cp.done)
		defer cancel()
		defer f.Close()
		c.runCapture(ctx, name, f)
	}()
	return nil
}

func (c *Client) runCapture(ctx context.Context, name string, w io.Writer) {
	backoff := captureBackoffMin
	for ctx.Err() == nil {
		dom, err := c.lv.DomainLookupByName(name)
		if err != nil {
			return
		}
		st, _, err := c.lv.DomainGetState(dom, 0)
		if err != nil || toState(st) == StateShutoff {
			return
		}

		began := time.Now()
		err = c.lv.DomainOpenConsole(dom, golibvirt.OptString{}, ctxWriter{ctx, w}, 0)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.log("console of %s: %v", name, err)
		}
		if time.Since(began) >= captureStable {
			backoff = captureBackoffMin
		}
		if sleepCtx(ctx, backoff) != nil {
			return
		}
		backoff = min(backoff*2, captureBackoffMax)
	}
}

// waitCapture waits, bounded by ctx and captureDrainWait, for the capture of a
// stopped domain to flush its last output and finish.
func (c *Client) waitCapture(ctx context.Context, name string) {
	c.caps.mu.Lock()
	cp := c.caps.m[name]
	c.caps.mu.Unlock()
	if cp == nil {
		return
	}
	t := time.NewTimer(captureDrainWait)
	defer t.Stop()
	select {
	case <-cp.done:
	case <-ctx.Done():
	case <-t.C:
		cp.cancel()
	}
}

// stopCaptures cancels every capture and waits for them to finish or ctx to
// end. Callers stop the domains first, which ends the console streams.
func (c *Client) stopCaptures(ctx context.Context) error {
	c.caps.mu.Lock()
	caps := make([]*capture, 0, len(c.caps.m))
	for _, cp := range c.caps.m {
		cp.cancel()
		caps = append(caps, cp)
	}
	c.caps.mu.Unlock()
	for _, cp := range caps {
		select {
		case <-cp.done:
		case <-ctx.Done():
			return fmt.Errorf("waiting for serial capture to stop: %w", ctx.Err())
		}
	}
	return nil
}
