package libvirt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	golibvirt "github.com/digitalocean/go-libvirt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestIntegration needs a reachable libvirt daemon (qemu:///system) and is
// skipped unless STORM_LIBVIRT_INTEGRATION=1. Everything it creates is named
// after a random namespace and removed again; nothing else is touched.
func TestIntegration(t *testing.T) {
	if os.Getenv("STORM_LIBVIRT_INTEGRATION") != "1" {
		t.Skip("set STORM_LIBVIRT_INTEGRATION=1 to run against a real libvirt")
	}

	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		t.Fatal(err)
	}
	ns := "stormtest-" + hex.EncodeToString(rnd[:])

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	c, err := Connect(ctx, "", Options{Namespace: ns, Logf: t.Logf})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	foreignBefore := listNames(t, c)

	// Deferred right after Connect so that a failing test cannot leak
	// resources, and so it verifies that nothing is left afterwards.
	defer func() {
		dctx, dcancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer dcancel()
		if err := c.Destroy(dctx); err != nil {
			t.Errorf("Destroy: %v", err)
		}
		if err := c.Destroy(dctx); err != nil {
			t.Errorf("second Destroy (should be a no-op): %v", err)
		}
		after := listNames(t, c)
		for kind, names := range after {
			for _, n := range names {
				if strings.HasPrefix(n, ns) {
					t.Errorf("leftover %s %q after Destroy", kind, n)
				}
			}
		}
		for kind, names := range foreignBefore {
			for _, n := range names {
				if !contains(after[kind], n) {
					t.Errorf("foreign %s %q disappeared", kind, n)
				}
			}
		}
		if _, err := os.Stat(c.sc.poolPath()); !os.IsNotExist(err) {
			t.Errorf("pool directory %s still present (err=%v)", c.sc.poolPath(), err)
		}
		// Destroy waits for the serial captures, so none may still be running.
		stacks := make([]byte, 1<<20)
		stacks = stacks[:runtime.Stack(stacks, true)]
		if bytes.Contains(stacks, []byte("(*Client).runCapture")) {
			t.Errorf("serial capture goroutine still running after Destroy:\n%s", stacks)
		}
	}()

	dir := t.TempDir()
	serial := filepath.Join(dir, "serial.log")
	// Local image files, used both as CDROM media and as ImagePath disks.
	rawImage := filepath.Join(dir, "raw.img")
	if err := os.WriteFile(rawImage, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	qcowImage := filepath.Join(dir, "disk.qcow2")
	if out, err := exec.Command("qemu-img", "create", "-f", "qcow2", qcowImage, "1G").CombinedOutput(); err != nil {
		t.Fatalf("qemu-img create: %v\n%s", err, out)
	}

	vm, err := c.Provision(ctx, Spec{
		Name:      "vm0",
		CPUs:      1,
		MemoryMiB: 512,
		Disks:     []Disk{{SizeGiB: 1, BootOrder: 1}},
		CDROMs:    []CDROM{{}},
		SerialLog: serial,
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if want := c.sc.domainName("vm0"); vm.Name() != want {
		t.Errorf("Name = %q, want %q", vm.Name(), want)
	}
	if vm.SerialLogPath() != serial || vm.MAC() == "" {
		t.Errorf("SerialLogPath/MAC = %q / %q", vm.SerialLogPath(), vm.MAC())
	}

	if _, err := c.Provision(ctx, Spec{Name: "vm0", CPUs: 1, MemoryMiB: 512, Disks: []Disk{{SizeGiB: 1}}}); err == nil {
		t.Error("provisioning a duplicate VM name succeeded")
	}

	expectState(ctx, t, vm, StateShutoff)
	if err := vm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	expectState(ctx, t, vm, StateRunning)
	if err := vm.Start(ctx); err != nil {
		t.Errorf("second Start should be a no-op: %v", err)
	}

	// The capture file belongs to the calling user and receives firmware
	// output; it must keep receiving output after a guest reset.
	st, err := os.Stat(serial)
	if err != nil {
		t.Fatalf("serial log: %v", err)
	}
	if uid := st.Sys().(*syscall.Stat_t).Uid; int(uid) != os.Getuid() {
		t.Errorf("serial log owned by uid %d, want %d", uid, os.Getuid())
	}
	if st.Mode().Perm() != 0o644 {
		t.Errorf("serial log mode = %v, want 0644", st.Mode().Perm())
	}
	waitFor(t, 60*time.Second, "firmware output in serial log", func() bool {
		b, err := os.ReadFile(serial)
		return err == nil && bytes.Contains(b, []byte("BdsDxe"))
	})
	before := fileSize(t, serial)
	t.Logf("serial log: %d bytes before reset", before)
	if err := vm.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	waitFor(t, 60*time.Second, "serial output after guest reset", func() bool {
		return fileSize(t, serial) > before
	})
	expectState(ctx, t, vm, StateRunning)
	t.Logf("serial log: %d bytes after reset", fileSize(t, serial))

	// An empty guest never asks for DHCP, so there is no lease to find.
	if _, err := vm.Address(ctx); !errors.Is(err, ErrNoAddress) {
		t.Errorf("Address on a guest without an OS = %v, want ErrNoAddress", err)
	}

	// CDROM media must be readable by the QEMU user and allowed by the host's
	// confinement policy, which a file in a home directory is not. Upload the
	// image into the namespace's pool (removed by Destroy) instead.
	pool, err := c.ensurePool(ctx)
	if err != nil {
		t.Fatalf("ensurePool: %v", err)
	}
	rawInfo, err := inspectImage(rawImage)
	if err != nil || rawInfo.format != formatRaw {
		t.Fatalf("inspectImage(raw) = %+v, %v", rawInfo, err)
	}
	iso, err := c.createVolume(ctx, pool, c.sc.prefix()+"media.img", formatRaw, 0, &rawInfo)
	if err != nil {
		t.Fatalf("create media volume: %v", err)
	}

	// SetCDROM on a running VM changes the live guest and the persistent
	// definition, so the medium is also there on the next boot.
	if err := vm.SetCDROM(ctx, 0, iso); err != nil {
		t.Errorf("SetCDROM insert: %v", err)
	}
	if !strings.Contains(domainXML(t, c, vm, true), iso) || !strings.Contains(domainXML(t, c, vm, false), iso) {
		t.Errorf("inserted medium missing from the live or persistent definition")
	}
	if err := vm.SetCDROM(ctx, 0, ""); err != nil {
		t.Errorf("SetCDROM eject: %v", err)
	}
	if strings.Contains(domainXML(t, c, vm, true), iso) || strings.Contains(domainXML(t, c, vm, false), iso) {
		t.Errorf("ejected medium still in the live or persistent definition")
	}
	if err := vm.SetCDROM(ctx, 1, ""); err == nil {
		t.Error("SetCDROM with out-of-range slot succeeded")
	}

	// A guest with no OS ignores ACPI power button presses, so a graceful
	// shutdown is expected to run into its deadline.
	sctx, scancel := context.WithTimeout(ctx, 5*time.Second)
	err = vm.Shutdown(sctx)
	scancel()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown: %v", err)
	}
	if err := vm.ForceStop(ctx); err != nil {
		t.Fatalf("ForceStop: %v", err)
	}
	expectState(ctx, t, vm, StateShutoff)

	// A second VM exercising several drives, secure boot firmware and a TPM.
	vm2, err := c.Provision(ctx, Spec{
		Name:       "vm1",
		CPUs:       1,
		MemoryMiB:  512,
		Disks:      []Disk{{ImagePath: qcowImage, SizeGiB: 2}, {ImagePath: rawImage}, {SizeGiB: 1, BootOrder: 2}},
		CDROMs:     []CDROM{{Path: iso, BootOrder: 1}, {}},
		SecureBoot: true,
		TPM:        true,
	})
	if err != nil {
		t.Fatalf("Provision vm1: %v", err)
	}
	// The copied qcow2 image was enlarged from its 1 GiB to the requested size.
	pool, err = c.ensureForTest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vol, err := c.lv.StorageVolLookupByName(pool, c.sc.volumeName("vm1", 0, formatQcow2))
	if err != nil {
		t.Fatalf("look up grown volume: %v", err)
	}
	if _, capacity, _, err := c.lv.StorageVolGetInfo(vol); err != nil || capacity != 2<<30 {
		t.Errorf("grown volume capacity = %d, %v; want %d", capacity, err, uint64(2<<30))
	}
	if err := vm2.Start(ctx); err != nil {
		t.Fatalf("Start vm1: %v", err)
	}
	expectState(ctx, t, vm2, StateRunning)
	if err := vm2.ForceStop(ctx); err != nil {
		t.Errorf("ForceStop vm1: %v", err)
	}
}

// TestIntegrationCoexist checks that two clients on one host do not interfere,
// even when one namespace is a prefix of the other.
func TestIntegrationCoexist(t *testing.T) {
	if os.Getenv("STORM_LIBVIRT_INTEGRATION") != "1" {
		t.Skip("set STORM_LIBVIRT_INTEGRATION=1 to run against a real libvirt")
	}
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		t.Fatal(err)
	}
	nsA := "stormtest-" + hex.EncodeToString(rnd[:])
	nsB := nsA + "-b"

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	var clients []*Client
	for _, ns := range []string{nsA, nsB} {
		c, err := Connect(ctx, "", Options{Namespace: ns, Logf: t.Logf})
		if err != nil {
			t.Fatalf("Connect %s: %v", ns, err)
		}
		defer c.Close()
		defer func() {
			dctx, dcancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer dcancel()
			if err := c.Destroy(dctx); err != nil {
				t.Errorf("Destroy %s: %v", c.sc, err)
			}
		}()
		if _, err := c.Provision(ctx, Spec{Name: "vm0", CPUs: 1, MemoryMiB: 256, Disks: []Disk{{SizeGiB: 1}}}); err != nil {
			t.Fatalf("Provision in %s: %v", ns, err)
		}
		clients = append(clients, c)
	}

	subnets := map[string]string{}
	for _, c := range clients {
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
		subnets[c.sc.ns] = ps[0].String()
	}
	if subnets[nsA] == subnets[nsB] {
		t.Errorf("both namespaces share subnet %s", subnets[nsA])
	}

	dctx, dcancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer dcancel()
	if err := clients[0].Destroy(dctx); err != nil {
		t.Fatalf("Destroy %s: %v", nsA, err)
	}
	names := listNames(t, clients[1])
	for kind, want := range map[string]string{
		"domain":  clients[1].sc.domainName("vm0"),
		"network": clients[1].sc.networkName(),
		"pool":    clients[1].sc.poolName(),
		"volume":  clients[1].sc.volumeName("vm0", 0, formatQcow2),
	} {
		if !contains(names[kind], want) {
			t.Errorf("destroying %s removed %s %q of %s", nsA, kind, want, nsB)
		}
	}
	for kind, list := range names {
		for _, n := range list {
			if clients[0].sc.owns(n) {
				t.Errorf("%s %q of %s survived its Destroy", kind, n, nsA)
			}
		}
	}
}

func (c *Client) ensureForTest(ctx context.Context) (golibvirt.StoragePool, error) {
	if err := c.acquire(ctx); err != nil {
		return golibvirt.StoragePool{}, err
	}
	defer c.release()
	return c.ensurePool(ctx)
}

// domainXML returns the live or persistent (inactive) definition of a VM.
func domainXML(t *testing.T, c *Client, vm *VM, live bool) string {
	t.Helper()
	dom, err := vm.domain()
	if err != nil {
		t.Fatal(err)
	}
	var flags golibvirt.DomainXMLFlags
	if !live {
		flags = golibvirt.DomainXMLInactive
	}
	xml, err := c.lv.DomainGetXMLDesc(dom, flags)
	if err != nil {
		t.Fatal(err)
	}
	return xml
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

func expectState(ctx context.Context, t *testing.T, vm *VM, want State) {
	t.Helper()
	got, err := vm.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if got != want {
		t.Fatalf("State = %q, want %q", got, want)
	}
}

func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("timed out waiting for %s", what)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// listNames returns the names of every domain, network, pool and volume
// libvirt knows about, keyed by kind.
func listNames(t *testing.T, c *Client) map[string][]string {
	t.Helper()
	out := map[string][]string{}

	doms, _, err := c.lv.ConnectListAllDomains(1, 3)
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	for _, d := range doms {
		out["domain"] = append(out["domain"], d.Name)
	}
	nets, _, err := c.lv.ConnectListAllNetworks(1, 3)
	if err != nil {
		t.Fatalf("list networks: %v", err)
	}
	for _, n := range nets {
		out["network"] = append(out["network"], n.Name)
	}
	pools, _, err := c.lv.ConnectListAllStoragePools(1, 0)
	if err != nil {
		t.Fatalf("list pools: %v", err)
	}
	for _, p := range pools {
		out["pool"] = append(out["pool"], p.Name)
		if active, err := c.lv.StoragePoolIsActive(p); err != nil || active == 0 {
			continue
		}
		vols, _, err := c.lv.StoragePoolListAllVolumes(p, 1, 0)
		if err != nil {
			continue
		}
		for _, v := range vols {
			out["volume"] = append(out["volume"], v.Name)
		}
	}
	return out
}
