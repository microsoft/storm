package libvirt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// integrationNamespace returns a fresh random namespace, and registers a final
// check that nothing carrying it is left on the host.
func integrationNamespace(t *testing.T) string {
	t.Helper()
	if os.Getenv("STORM_LIBVIRT_INTEGRATION") != "1" {
		t.Skip("set STORM_LIBVIRT_INTEGRATION=1 to run against a real libvirt")
	}
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		t.Fatal(err)
	}
	ns := "stormtest-" + hex.EncodeToString(rnd[:])

	c, err := Connect(context.Background(), "", Options{Namespace: ns})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer c.Close()
		for kind, names := range listNames(t, c) {
			for _, n := range names {
				if strings.HasPrefix(n, ns) {
					t.Errorf("leftover %s %q", kind, n)
				}
			}
		}
	})
	return ns
}

// connectRun connects a client for ns (taking over runID when set) that is
// destroyed and closed when the test ends.
func connectRun(ctx context.Context, t *testing.T, ns, runID string) *Client {
	t.Helper()
	c, err := Connect(ctx, "", Options{Namespace: ns, RunID: runID, Logf: t.Logf})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if c.lv.IsConnected() {
			if err := c.Destroy(dctx); err != nil {
				t.Errorf("Destroy %s: %v", c.sc, err)
			}
		}
		c.Close()
	})
	return c
}

func tinySpec(name string) Spec {
	return Spec{Name: name, CPUs: 1, MemoryMiB: 256, Disks: []Disk{{SizeGiB: 1}}}
}

func subnetOf(t *testing.T, c *Client) string {
	t.Helper()
	nw, err := c.lv.NetworkLookupByName(c.sc.networkName())
	if err != nil {
		t.Fatal(err)
	}
	xml, err := c.lv.NetworkGetXMLDesc(nw, 0)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := networkPrefixes(xml)
	if err != nil || len(ps) != 1 {
		t.Fatalf("prefixes of %s: %v, %v", c.sc, ps, err)
	}
	return ps[0].String()
}

func ownedBy(c *Client, names map[string][]string) []string {
	var out []string
	for kind, list := range names {
		for _, n := range list {
			if c.sc.owns(n) {
				out = append(out, kind+" "+n)
			}
		}
	}
	slices.Sort(out)
	return out
}

// TestIntegrationRuns checks that clients sharing a namespace are isolated
// from each other, that the network and pool are created once per run, that
// Destroy honours its context, and that the objects of a crashed run can be
// found and removed.
func TestIntegrationRuns(t *testing.T) {
	ns := integrationNamespace(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	a := connectRun(ctx, t, ns, "")
	b := connectRun(ctx, t, ns, "")
	if a.RunID() == b.RunID() {
		t.Fatalf("two fresh clients share run ID %s", a.RunID())
	}

	for _, c := range []*Client{a, b} {
		if _, err := c.Provision(ctx, tinySpec("vm0")); err != nil {
			t.Fatalf("Provision in %s: %v", c.sc, err)
		}
	}
	if sa, sb := subnetOf(t, a), subnetOf(t, b); sa == sb {
		t.Errorf("both runs share subnet %s", sa)
	}

	// A second VM in the same run must reuse the network, not recreate it.
	nw, err := a.lv.NetworkLookupByName(a.sc.networkName())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Provision(ctx, tinySpec("vm1")); err != nil {
		t.Fatalf("second Provision: %v", err)
	}
	nw2, err := a.lv.NetworkLookupByName(a.sc.networkName())
	if err != nil || nw2.UUID != nw.UUID {
		t.Errorf("network was recreated by the second Provision (%v)", err)
	}

	// A cancelled context stops Destroy before it removes anything.
	cctx, ccancel := context.WithCancel(ctx)
	ccancel()
	before := ownedBy(a, listNames(t, a))
	if err := a.Destroy(cctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Destroy with a cancelled context = %v, want context.Canceled", err)
	}
	if after := ownedBy(a, listNames(t, a)); !slices.Equal(before, after) {
		t.Errorf("Destroy with a cancelled context changed the host:\nbefore %v\nafter  %v", before, after)
	}

	// Destroying run B leaves run A, in the same namespace, untouched.
	aBefore := ownedBy(a, listNames(t, a))
	if err := b.Destroy(ctx); err != nil {
		t.Fatalf("Destroy b: %v", err)
	}
	names := listNames(t, a)
	if got := ownedBy(b, names); len(got) != 0 {
		t.Errorf("run b left %v", got)
	}
	if got := ownedBy(a, names); !slices.Equal(got, aBefore) {
		t.Errorf("destroying run b changed run a:\nbefore %v\nafter  %v", aBefore, got)
	}
	// Destroy is idempotent and the client can provision again afterwards.
	if err := b.Destroy(ctx); err != nil {
		t.Errorf("second Destroy b: %v", err)
	}
	if _, err := b.Provision(ctx, tinySpec("vm0")); err != nil {
		t.Errorf("Provision after Destroy: %v", err)
	}

	// A fresh run never silently adopts an existing run's network or pool.
	f := connectRun(ctx, t, ns, "")
	f.sc = a.sc
	if _, err := f.Provision(ctx, tinySpec("other")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("fresh client adopted an existing run: %v", err)
	}
	f.sc = scope{ns: ns, run: f.sc.run + "x"} // nothing to destroy for the cleanup

	// Crash recovery: the process owning run A goes away without Destroy.
	runA := a.RunID()
	a.Close()
	c := connectRun(ctx, t, ns, runA)
	runs, err := c.Runs(ctx)
	if err != nil || !slices.Contains(runs, runA) || !slices.Contains(runs, b.RunID()) {
		t.Errorf("Runs = %v, %v; want it to include %s and %s", runs, err, runA, b.RunID())
	}
	if err := c.Destroy(ctx); err != nil {
		t.Fatalf("Destroy of taken-over run: %v", err)
	}
	names = listNames(t, c)
	if got := ownedBy(c, names); len(got) != 0 {
		t.Errorf("taken-over run left %v", got)
	}
	if got := ownedBy(b, names); len(got) == 0 {
		t.Error("destroying the taken-over run removed run b's objects")
	}
}

// TestIntegrationParallelSubnets provisions from several clients at once: the
// networks they create must not overlap.
func TestIntegrationParallelSubnets(t *testing.T) {
	ns := integrationNamespace(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const n = 5
	clients := make([]*Client, n)
	for i := range clients {
		clients[i] = connectRun(ctx, t, ns, "")
	}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.Provision(ctx, tinySpec("vm0"))
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Provision %d: %v", i, err)
		}
	}
	seen := map[string]string{}
	for _, c := range clients {
		s := subnetOf(t, c)
		if other, dup := seen[s]; dup {
			t.Errorf("runs %s and %s share subnet %s", other, c.sc, s)
		}
		seen[s] = c.sc.String()
	}
}
