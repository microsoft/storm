// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"sync"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

// State is the coarse run state of a VM.
type State string

const (
	StateRunning State = "running"
	StateShutoff State = "shutoff"
	// StateOther covers every other libvirt state (paused, shutting down,
	// crashed, ...).
	StateOther State = "other"
)

// shutdownPollInterval is how often Shutdown checks whether the guest has
// powered off.
const shutdownPollInterval = 500 * time.Millisecond

// VM is a handle to a provisioned domain. Methods are safe for concurrent use.
type VM struct {
	c *Client

	mu   sync.Mutex // guards plan.cdroms
	plan domainPlan

	leases    leaseQuery
	stabilize stabilizeConfig
}

// Name returns the libvirt domain name, "<namespace>_<Spec.Name>", as shown by
// virsh.
func (v *VM) Name() string { return v.plan.name }

// MAC returns the MAC address of the VM's network interface.
func (v *VM) MAC() string { return v.plan.mac }

// SerialLogPath returns the path of the serial console capture file, or "" when
// none was requested.
func (v *VM) SerialLogPath() string { return v.plan.serialLog }

func (v *VM) domain() (golibvirt.Domain, error) {
	dom, err := v.c.lv.DomainLookupByName(v.plan.name)
	if err != nil {
		return dom, fmt.Errorf("look up domain %s: %w", v.plan.name, err)
	}
	return dom, nil
}

func toState(s int32) State {
	switch golibvirt.DomainState(s) {
	case golibvirt.DomainRunning:
		return StateRunning
	case golibvirt.DomainShutoff:
		return StateShutoff
	default:
		return StateOther
	}
}

// State reports the current run state.
func (v *VM) State(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dom, err := v.domain()
	if err != nil {
		return "", err
	}
	st, _, err := v.c.lv.DomainGetState(dom, 0)
	if err != nil {
		return "", fmt.Errorf("get state of %s: %w", v.plan.name, err)
	}
	return toState(st), nil
}

// Start boots the VM and, when [Spec.SerialLog] is set, begins capturing the
// serial console. Starting a running VM only makes sure the capture runs.
func (v *VM) Start(ctx context.Context) error {
	st, err := v.State(ctx)
	if err != nil {
		return err
	}
	if st == StateRunning {
		return v.c.startCapture(v.plan.name, v.plan.serialLog)
	}
	dom, err := v.domain()
	if err != nil {
		return err
	}
	if err := v.c.lv.DomainCreate(dom); err != nil {
		return fmt.Errorf("start %s: %w", v.plan.name, err)
	}
	if err := v.c.startCapture(v.plan.name, v.plan.serialLog); err != nil {
		return fmt.Errorf("%s started but %w", v.plan.name, err)
	}
	return nil
}

// Reset hard-resets a running guest, like pressing the reset button. The
// domain keeps running, and so does the serial capture.
func (v *VM) Reset(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dom, err := v.domain()
	if err != nil {
		return err
	}
	if err := v.c.lv.DomainReset(dom, 0); err != nil {
		return fmt.Errorf("reset %s: %w", v.plan.name, err)
	}
	return nil
}

// Shutdown asks the guest to power off through ACPI and waits until the domain
// is shut off, so that callers can safely use the VM's disks afterwards. It
// returns nil only once the VM is off. If ctx ends first it returns an error
// wrapping the context's error and leaves the VM as it is: Shutdown never
// forces the VM off. A guest that ignores ACPI never shuts down, so pass a
// context with a timeout and decide yourself whether to call [VM.ForceStop].
func (v *VM) Shutdown(ctx context.Context) error {
	st, err := v.State(ctx)
	if err != nil {
		return err
	}
	if st == StateShutoff {
		return nil
	}
	dom, err := v.domain()
	if err != nil {
		return err
	}
	if err := v.c.lv.DomainShutdown(dom); err != nil && !hasLibvirtCode(err, golibvirt.ErrOperationInvalid) {
		return fmt.Errorf("shut down %s: %w", v.plan.name, err)
	}
	for {
		if err := sleepCtx(ctx, shutdownPollInterval); err != nil {
			return fmt.Errorf("waiting for %s to shut down: %w", v.plan.name, err)
		}
		st, err := v.State(ctx)
		if err != nil {
			return err
		}
		if st == StateShutoff {
			v.c.waitCapture(ctx, v.plan.name)
			return nil
		}
	}
}

// ForceStop powers the VM off immediately, like pulling the plug. The domain
// definition is kept. Stopping a VM that is not running is a no-op.
func (v *VM) ForceStop(ctx context.Context) error {
	st, err := v.State(ctx)
	if err != nil {
		return err
	}
	if st == StateShutoff {
		return nil
	}
	dom, err := v.domain()
	if err != nil {
		return err
	}
	if err := v.c.lv.DomainDestroy(dom); err != nil && !hasLibvirtCode(err, golibvirt.ErrOperationInvalid) {
		return fmt.Errorf("force stop %s: %w", v.plan.name, err)
	}
	v.c.waitCapture(ctx, v.plan.name)
	return nil
}

// Address returns the VM's IPv4 address from the network's DHCP leases. It
// makes one lookup (plus a bounded number of retries when several leases
// exist for the VM, which happens briefly when a guest sends duplicate DHCP
// requests) and never waits for a lease to appear: while there is none it
// returns an error wrapping [ErrNoAddress], and the caller is expected to
// poll.
func (v *VM) Address(ctx context.Context) (netip.Addr, error) {
	addr, err := resolveAddress(ctx, v.leases, v.stabilize)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("address of %s: %w", v.plan.name, err)
	}
	return addr, nil
}

func (v *VM) libvirtLeases(ctx context.Context) ([]netip.Addr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dom, err := v.domain()
	if err != nil {
		return nil, err
	}
	ifaces, err := v.c.lv.DomainInterfaceAddresses(dom, uint32(golibvirt.DomainInterfaceAddressesSrcLease), 0)
	if err != nil {
		return nil, err
	}
	return ipv4Leases(ifaces, v.plan.mac), nil
}

// SetCDROM inserts the ISO at path (an absolute path on the libvirt host) into
// the CDROM slot, or ejects the medium when path is empty. Slots are indexed
// in the order of [Spec.CDROMs].
//
// The change is made to the persistent definition, so the new medium (or the
// empty bay) is also what the VM has on its next boot, including after a
// shutdown and [VM.Start]. When the VM is running, the same change is applied
// to the live guest at the same time. Ejecting a running guest's medium is
// forced so that a tray locked by the guest does not block it. The drive keeps
// its boot order.
func (v *VM) SetCDROM(ctx context.Context, slot int, path string) error {
	if path != "" && !filepath.IsAbs(path) {
		return fmt.Errorf("CDROM path %q must be absolute", path)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if slot < 0 || slot >= len(v.plan.cdroms) {
		return fmt.Errorf("CDROM slot %d out of range (VM has %d)", slot, len(v.plan.cdroms))
	}
	st, err := v.State(ctx)
	if err != nil {
		return err
	}
	dom, err := v.domain()
	if err != nil {
		return err
	}

	cd := v.plan.cdroms[slot]
	cd.path = path
	d := cdromDisk(cd)
	xml, err := d.Marshal()
	if err != nil {
		return fmt.Errorf("marshal CDROM XML: %w", err)
	}

	flags := golibvirt.DomainDeviceModifyConfig
	if st != StateShutoff {
		flags |= golibvirt.DomainDeviceModifyLive
		if path == "" {
			flags |= golibvirt.DomainDeviceModifyForce
		}
	}
	if err := v.c.lv.DomainUpdateDeviceFlags(dom, xml, flags); err != nil {
		return fmt.Errorf("update CDROM %d of %s: %w", slot, v.plan.name, err)
	}
	v.plan.cdroms[slot] = cd
	return nil
}
