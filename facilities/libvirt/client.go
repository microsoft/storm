// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"

	golibvirt "github.com/digitalocean/go-libvirt"
)

// Options configures a [Client].
type Options struct {
	// Namespace groups runs and prefixes the names of everything a client
	// creates, which makes them recognisable on a shared host. Required:
	// lowercase letters, digits and single hyphens, at most 32 characters.
	// Namespaces do not isolate anything by themselves; see RunID.
	Namespace string

	// RunID identifies the run that owns the created objects: lowercase letters
	// and digits, at most 16 characters. Leave it empty to start a new run
	// with a random ID; every object then carries an ID no one else has, and a
	// client never touches objects of another run, even in the same namespace.
	//
	// Setting it is an explicit opt-in to take over the objects of an
	// existing run, for example to destroy what a crashed process left behind
	// (see [Client.Runs]). Only then may Provision reuse a network and storage
	// pool that already exist.
	RunID string

	// Logf receives diagnostic messages. Nil means silent.
	Logf func(format string, args ...any)
}

// Client talks to one libvirt daemon on behalf of one run. Connection
// lifetime and resource ownership are separate: [Client.Close] only
// disconnects, and [Client.Destroy] removes what the run created.
type Client struct {
	lv      *golibvirt.Libvirt
	sc      scope
	adopted bool // the caller supplied the run ID
	logf    func(format string, args ...any)

	// undefineTPM is set when libvirt supports removing TPM state on undefine.
	undefineTPM bool

	// lock serialises Provision and Destroy, which create and remove the
	// shared per-run network and pool. It is a channel so waiting honours
	// contexts.
	lock chan struct{}
	// netReady and poolReady record that this client created (or took over)
	// the shared objects, so later Provision calls do not look them up again.
	// Guarded by lock.
	netReady, poolReady bool

	caps captures
}

// minTPMUndefineVersion is the libvirt version (8.9.0) that introduced
// VIR_DOMAIN_UNDEFINE_TPM.
const minTPMUndefineVersion = 8009000

const undefineTPM golibvirt.DomainUndefineFlagsValues = 32

// Connect opens a connection to libvirt. An empty uri means qemu:///system.
//
// The connection attempt itself is not interruptible; if ctx ends first,
// Connect returns promptly and a helper goroutine closes the connection as
// soon as the attempt finishes.
func Connect(ctx context.Context, uri string, o Options) (*Client, error) {
	if err := validateName("namespace", o.Namespace); err != nil {
		return nil, err
	}
	run, adopted := o.RunID, o.RunID != ""
	if adopted {
		if err := validateRunID(run); err != nil {
			return nil, err
		}
	} else {
		var err error
		if run, err = newRunID(); err != nil {
			return nil, err
		}
	}
	if uri == "" {
		uri = string(golibvirt.QEMUSystem)
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse libvirt URI %q: %w", uri, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type result struct {
		lv  *golibvirt.Libvirt
		err error
	}
	ch := make(chan result, 1)
	go func() {
		lv, err := golibvirt.ConnectToURI(u)
		ch <- result{lv, err}
	}()

	var lv *golibvirt.Libvirt
	select {
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.lv != nil {
				_ = r.lv.Disconnect()
			}
		}()
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("connect to libvirt at %q: %w", uri, r.err)
		}
		lv = r.lv
	}

	c := &Client{
		lv:      lv,
		sc:      scope{ns: o.Namespace, run: run},
		adopted: adopted,
		logf:    o.Logf,
		lock:    make(chan struct{}, 1),
	}
	c.caps.init()
	if ver, err := lv.ConnectGetLibVersion(); err == nil {
		c.undefineTPM = ver >= minTPMUndefineVersion
	}
	return c, nil
}

// Namespace returns the namespace of the client.
func (c *Client) Namespace() string { return c.sc.ns }

// RunID returns the ID of the run the client owns. Record it (for example in
// a log or artifact) to be able to clean up after a crash by connecting again
// with [Options.RunID].
func (c *Client) RunID() string { return c.sc.run }

// Close disconnects from libvirt. It never removes or stops anything; call
// [Client.Destroy] for that. Serial console captures are tied to the
// connection and end with it, so Close stops them and waits for their
// goroutines.
func (c *Client) Close() error {
	c.caps.shutdown()
	var err error
	if c.lv != nil && c.lv.IsConnected() {
		// Disconnecting also ends console streams that are waiting for output.
		err = c.lv.Disconnect()
	}
	c.caps.wg.Wait()
	return err
}

// Runs lists the IDs of all runs in the client's namespace that have objects
// (domains, networks or storage pools) on the host, including the client's own
// run. It is meant for recovering from crashes: connect again with each
// leftover ID in [Options.RunID] and call [Client.Destroy].
func (c *Client) Runs(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var names []string
	doms, _, err := c.lv.ConnectListAllDomains(1, golibvirt.ConnectListDomainsActive|golibvirt.ConnectListDomainsInactive)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	for _, d := range doms {
		names = append(names, d.Name)
	}
	nets, _, err := c.lv.ConnectListAllNetworks(1, golibvirt.ConnectListNetworksActive|golibvirt.ConnectListNetworksInactive)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	for _, n := range nets {
		names = append(names, n.Name)
	}
	pools, _, err := c.lv.ConnectListAllStoragePools(1, 0)
	if err != nil {
		return nil, fmt.Errorf("list storage pools: %w", err)
	}
	for _, p := range pools {
		names = append(names, p.Name)
	}
	runs := runsIn(c.sc.ns, names)
	slices.Sort(runs)
	return runs, nil
}

func (c *Client) log(format string, args ...any) {
	if c.logf != nil {
		c.logf(format, args...)
	}
}

// acquire takes the client's lock or fails when ctx ends first.
func (c *Client) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.lock <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() { <-c.lock }

func hasLibvirtCode(err error, codes ...golibvirt.ErrorNumber) bool {
	var le golibvirt.Error
	if !errors.As(err, &le) {
		return false
	}
	for _, code := range codes {
		if le.Code == uint32(code) {
			return true
		}
	}
	return false
}
