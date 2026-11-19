// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

// rollbackTimeout bounds the cleanup after a failed Provision. The cleanup uses
// its own context so that it still runs when the failure was a cancellation.
const rollbackTimeout = 30 * time.Second

// networkAttempts is how many subnets ensureNetwork tries when creating the
// network fails or collides with another network (for example one a parallel
// process created at the same moment).
const networkAttempts = 5

// Provision defines a VM from s, creating the run's shared NAT network and
// storage pool on the first call and reusing them afterwards. The VM is
// defined but not started; see [VM.Start]. VM names must be unique within the
// run.
//
// Before anything is created, Provision checks that the libvirt host can
// provide the requested architecture, firmware and devices; an unsupported
// SecureBoot or TPM request is an error, never a silent downgrade.
//
// If Provision fails, the objects created for this VM are removed with a
// fresh bounded context and any failure of that cleanup is returned together
// with the original error ([errors.Join]). The shared network and pool stay
// until [Client.Destroy].
//
// Provision calls and Destroy on one Client are serialised.
func (c *Client) Provision(ctx context.Context, s Spec) (vm *VM, err error) {
	s = s.withDefaults()
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	images := make([]*imageInfo, len(s.Disks))
	for i, d := range s.Disks {
		if d.ImagePath == "" {
			continue
		}
		info, err := inspectImage(d.ImagePath)
		if err != nil {
			return nil, fmt.Errorf("disk %d: %w", i, err)
		}
		if d.SizeGiB != 0 && uint64(d.SizeGiB)<<30 < info.capacity {
			return nil, fmt.Errorf("disk %d: SizeGiB %d is smaller than the %d byte virtual size of %s; shrinking is not supported",
				i, d.SizeGiB, info.capacity, d.ImagePath)
		}
		images[i] = &info
	}
	fw, err := findFirmware(s.Arch, s.SecureBoot, fileExists)
	if err != nil {
		return nil, err
	}

	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	if err := c.checkHost(targetFor(s.Arch), requirements{secureBoot: s.SecureBoot, tpm: s.TPM}); err != nil {
		return nil, err
	}

	domName := c.sc.domainName(s.Name)
	if _, err := c.lv.DomainLookupByName(domName); err == nil {
		return nil, fmt.Errorf("domain %s already exists", domName)
	} else if !hasLibvirtCode(err, golibvirt.ErrNoDomain) {
		return nil, fmt.Errorf("look up domain %s: %w", domName, err)
	}

	if err := c.ensureNetwork(ctx); err != nil {
		return nil, err
	}
	pool, err := c.ensurePool(ctx)
	if err != nil {
		return nil, err
	}

	var volNames []string
	domainDefined := false
	defer func() {
		if err == nil {
			return
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rerr := c.rollbackVM(rctx, pool, domName, domainDefined, volNames); rerr != nil {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rerr))
		}
	}()

	vols := make([]diskPlan, len(s.Disks))
	for i, d := range s.Disks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		format, capacity := formatQcow2, uint64(d.SizeGiB)<<30
		if images[i] != nil {
			format = images[i].format
			capacity = max(capacity, images[i].capacity)
		}
		name := c.sc.volumeName(s.Name, i, format)
		volNames = append(volNames, name)
		path, err := c.createVolume(ctx, pool, name, format, capacity, images[i])
		if err != nil {
			return nil, fmt.Errorf("disk %d: %w", i, err)
		}
		vols[i] = diskPlan{path: path, format: format}
	}

	mac, err := randomMAC()
	if err != nil {
		return nil, err
	}
	plan := newDomainPlan(domName, c.sc.networkName(), mac, s, vols, fw)
	dom := buildDomain(plan)
	xml, err := dom.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal domain XML: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := c.lv.DomainDefineXMLFlags(xml, golibvirt.DomainDefineValidate); err != nil {
		return nil, fmt.Errorf("define domain %s: %w", domName, err)
	}
	domainDefined = true
	c.log("defined domain %s (MAC %s)", domName, mac)

	v := &VM{c: c, plan: plan}
	v.leases = v.libvirtLeases
	v.stabilize = defaultStabilize
	return v, nil
}

// rollbackVM removes the objects created for a VM whose provisioning failed:
// the domain first, then its volumes. It keeps going after a failure and
// returns all failures.
func (c *Client) rollbackVM(ctx context.Context, pool golibvirt.StoragePool, domName string, domainDefined bool, volNames []string) error {
	var errs []error
	if domainDefined {
		if dom, err := c.lv.DomainLookupByName(domName); err == nil {
			if _, err := c.removeDomain(dom); err != nil {
				errs = append(errs, err)
			}
		} else if !hasLibvirtCode(err, golibvirt.ErrNoDomain) {
			errs = append(errs, fmt.Errorf("look up domain %s: %w", domName, err))
		}
	}
	for _, n := range volNames {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := c.deleteVolumeByName(pool, n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func randomMAC() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate MAC address: %w", err)
	}
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", b[0], b[1], b[2]), nil
}

// errSubnetConflict reports that another network overlaps the one just
// created.
var errSubnetConflict = errors.New("subnet overlaps another network")

// ensureNetwork makes sure the run's NAT network exists and is active. It is
// created once per run: later calls return immediately. A network that already
// exists is only accepted when the run was taken over through [Options.RunID].
//
// The subnet comes from chooseSubnet. Choosing is racy when other processes
// create networks at the same time, so a new network is verified after it has
// been started: if any other network overlaps it, it is removed again and
// another subnet is tried, after a short random delay.
func (c *Client) ensureNetwork(ctx context.Context) error {
	if c.netReady {
		return nil
	}
	name := c.sc.networkName()
	nw, err := c.lv.NetworkLookupByName(name)
	if err == nil {
		if !c.adopted {
			return fmt.Errorf("network %s already exists and this client did not create it (set Options.RunID to take over an existing run)", name)
		}
		active, err := c.lv.NetworkIsActive(nw)
		if err != nil {
			return fmt.Errorf("check network %s: %w", name, err)
		}
		if active == 0 {
			if err := c.lv.NetworkCreate(nw); err != nil {
				return fmt.Errorf("start network %s: %w", name, err)
			}
		}
		c.netReady = true
		return nil
	}
	if !hasLibvirtCode(err, golibvirt.ErrNoNetwork) {
		return fmt.Errorf("look up network %s: %w", name, err)
	}

	var avoid []netip.Prefix
	var lastErr error
	for attempt := 0; attempt < networkAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, jitter(50*time.Millisecond, 250*time.Millisecond)); err != nil {
				return err
			}
		}
		used, err := c.usedSubnets("", true)
		if err != nil {
			return err
		}
		subnet, err := chooseSubnet(c.sc.String(), append(used, avoid...))
		if err != nil {
			return err
		}
		lastErr = c.createNetwork(name, subnet)
		if lastErr == nil {
			c.log("created network %s on %s", name, subnet)
			c.netReady = true
			return nil
		}
		c.log("network %s on %s failed: %v", name, subnet, lastErr)
		avoid = append(avoid, subnet)
	}
	return fmt.Errorf("create network %s: %w", name, lastErr)
}

// createNetwork defines, starts and verifies the network. On any failure it
// removes what it created.
func (c *Client) createNetwork(name string, subnet netip.Prefix) (err error) {
	xml, err := networkXML(name, subnet)
	if err != nil {
		return fmt.Errorf("marshal network XML: %w", err)
	}
	nw, err := c.lv.NetworkDefineXML(xml)
	if err != nil {
		return fmt.Errorf("define network %s: %w", name, err)
	}
	defer func() {
		if err != nil {
			_ = c.lv.NetworkDestroy(nw)
			_ = c.lv.NetworkUndefine(nw)
		}
	}()
	if err := c.lv.NetworkCreate(nw); err != nil {
		return fmt.Errorf("start network %s: %w", name, err)
	}
	if err := c.lv.NetworkSetAutostart(nw, 1); err != nil {
		return fmt.Errorf("set network %s to autostart: %w", name, err)
	}
	others, err := c.usedSubnets(name, false)
	if err != nil {
		return err
	}
	if overlapsAny(subnet, others) {
		return fmt.Errorf("%w: %s", errSubnetConflict, subnet)
	}
	return nil
}

// usedSubnets collects the subnets of every libvirt network except the one
// named exclude and, with host set, of every local interface.
func (c *Client) usedSubnets(exclude string, host bool) ([]netip.Prefix, error) {
	nets, _, err := c.lv.ConnectListAllNetworks(1, golibvirt.ConnectListNetworksActive|golibvirt.ConnectListNetworksInactive)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	var used []netip.Prefix
	if host {
		used = hostPrefixes()
	}
	for _, nw := range nets {
		if nw.Name == exclude {
			continue
		}
		xml, err := c.lv.NetworkGetXMLDesc(nw, 0)
		if err != nil {
			if hasLibvirtCode(err, golibvirt.ErrNoNetwork) {
				continue
			}
			return nil, fmt.Errorf("describe network %s: %w", nw.Name, err)
		}
		ps, err := networkPrefixes(xml)
		if err != nil {
			return nil, fmt.Errorf("network %s: %w", nw.Name, err)
		}
		used = append(used, ps...)
	}
	return used, nil
}

// jitter returns a random duration in [lo, hi].
func jitter(lo, hi time.Duration) time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo)+1))
	if err != nil {
		return lo
	}
	return lo + time.Duration(n.Int64())
}
