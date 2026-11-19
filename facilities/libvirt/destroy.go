// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"context"
	"errors"
	"fmt"

	golibvirt "github.com/digitalocean/go-libvirt"
)

// Destroy removes everything the client's run owns: its domains (stopped if
// running, with their NVRAM and TPM state), the volumes of its storage pool,
// the pool itself, and its NAT network. Only objects named with the run's own
// prefix, "<namespace>_<run ID>_", are touched; objects of other runs in the
// same namespace, and anything else on the host, are left alone. Objects that
// are already gone are ignored, and a failed Destroy can simply be called
// again.
//
// The order is domains, then volumes and pool, then network. If a domain could
// not be stopped, its disks and network are left in place and the failure is
// returned. Otherwise Destroy keeps going after an individual failure and
// returns all failures joined. A pool whose volumes could not all be deleted
// is kept so that a retry still finds them. The [Spec.SerialLog] files are
// left alone; serial captures are stopped.
//
// Destroy honours ctx like every other method, and uses no deadline of its
// own. To clean up after a cancelled or timed-out run, pass a context that
// outlives it with its own budget:
//
//	ctx, cancel := context.WithTimeout(context.WithoutCancel(runCtx), 2*time.Minute)
//	defer cancel()
//	err := c.Destroy(ctx)
func (c *Client) Destroy(ctx context.Context) error {
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.release()

	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	finish := func() error { return errors.Join(errs...) }

	domains, _, err := c.lv.ConnectListAllDomains(1, golibvirt.ConnectListDomainsActive|golibvirt.ConnectListDomainsInactive)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}
	stuck := false
	for _, dom := range domains {
		if !c.sc.owns(dom.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			add(err)
			return finish()
		}
		running, err := c.removeDomain(dom)
		add(err)
		stuck = stuck || running
	}
	add(c.stopCaptures(ctx))
	if stuck {
		add(errors.New("a domain is still running; keeping its volumes, pool and network"))
		return finish()
	}

	pools, _, err := c.lv.ConnectListAllStoragePools(1, 0)
	if err != nil {
		add(fmt.Errorf("list storage pools: %w", err))
		return finish()
	}
	poolsOK := true
	for _, pool := range pools {
		if !c.sc.owns(pool.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			add(err)
			return finish()
		}
		if err := c.removePool(ctx, pool); err != nil {
			add(err)
			poolsOK = false
		}
	}
	if poolsOK {
		c.poolReady = false
	}

	nets, _, err := c.lv.ConnectListAllNetworks(1, golibvirt.ConnectListNetworksActive|golibvirt.ConnectListNetworksInactive)
	if err != nil {
		add(fmt.Errorf("list networks: %w", err))
		return finish()
	}
	netsOK := true
	for _, nw := range nets {
		if !c.sc.owns(nw.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			add(err)
			return finish()
		}
		if err := c.removeNetwork(nw); err != nil {
			add(err)
			netsOK = false
		}
	}
	if netsOK {
		c.netReady = false
	}
	return finish()
}

// removeDomain stops and undefines a domain along with its NVRAM and TPM
// state, snapshots metadata and managed-save image. Callers check ownership.
// The boolean reports that the domain is still running afterwards.
func (c *Client) removeDomain(dom golibvirt.Domain) (stillRunning bool, err error) {
	active, err := c.lv.DomainIsActive(dom)
	if err != nil {
		if hasLibvirtCode(err, golibvirt.ErrNoDomain) {
			return false, nil
		}
		return true, fmt.Errorf("check domain %s: %w", dom.Name, err)
	}
	if active != 0 {
		if err := c.lv.DomainDestroy(dom); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoDomain, golibvirt.ErrOperationInvalid) {
			return true, fmt.Errorf("stop domain %s: %w", dom.Name, err)
		}
	}
	flags := golibvirt.DomainUndefineNvram | golibvirt.DomainUndefineManagedSave | golibvirt.DomainUndefineSnapshotsMetadata | golibvirt.DomainUndefineCheckpointsMetadata
	if c.undefineTPM {
		flags |= undefineTPM
	}
	if err := c.lv.DomainUndefineFlags(dom, flags); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoDomain) {
		return false, fmt.Errorf("undefine domain %s: %w", dom.Name, err)
	}
	c.log("removed domain %s", dom.Name)
	return false, nil
}

// removePool deletes the run-owned volumes in a pool, then the pool and its
// directory. If any volume cannot be deleted, or the volumes cannot be listed,
// the pool is kept so that a retry can find what is left.
func (c *Client) removePool(ctx context.Context, pool golibvirt.StoragePool) error {
	active, err := c.lv.StoragePoolIsActive(pool)
	if err != nil {
		if hasLibvirtCode(err, golibvirt.ErrNoStoragePool) {
			return nil
		}
		return fmt.Errorf("check storage pool %s: %w", pool.Name, err)
	}
	if active == 0 {
		// Volumes can only be listed on an active pool.
		if err := c.lv.StoragePoolCreate(pool, 0); err != nil {
			return fmt.Errorf("start storage pool %s to delete its volumes: %w", pool.Name, err)
		}
	}

	var errs []error
	vols, _, err := c.lv.StoragePoolListAllVolumes(pool, 1, 0)
	if err != nil {
		return fmt.Errorf("list volumes in %s: %w", pool.Name, err)
	}
	for _, vol := range vols {
		if !c.sc.owns(vol.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := c.lv.StorageVolDelete(vol, 0); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoStorageVol) {
			errs = append(errs, fmt.Errorf("delete volume %s: %w", vol.Name, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if err := c.lv.StoragePoolDestroy(pool); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoStoragePool, golibvirt.ErrOperationInvalid) {
		return fmt.Errorf("stop storage pool %s: %w", pool.Name, err)
	}
	// Removing the (now empty) directory is best effort: a pool directory
	// that still holds foreign files is left in place.
	if err := c.lv.StoragePoolDelete(pool, 0); err != nil {
		c.log("storage pool %s: directory not removed: %v", pool.Name, err)
	}
	if err := c.lv.StoragePoolUndefine(pool); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoStoragePool) {
		return fmt.Errorf("undefine storage pool %s: %w", pool.Name, err)
	}
	c.log("removed storage pool %s", pool.Name)
	return nil
}

func (c *Client) removeNetwork(nw golibvirt.Network) error {
	active, err := c.lv.NetworkIsActive(nw)
	if err != nil {
		if hasLibvirtCode(err, golibvirt.ErrNoNetwork) {
			return nil
		}
		return fmt.Errorf("check network %s: %w", nw.Name, err)
	}
	if active != 0 {
		if err := c.lv.NetworkDestroy(nw); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoNetwork) {
			return fmt.Errorf("stop network %s: %w", nw.Name, err)
		}
	}
	if err := c.lv.NetworkUndefine(nw); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoNetwork) {
		return fmt.Errorf("undefine network %s: %w", nw.Name, err)
	}
	c.log("removed network %s", nw.Name)
	return nil
}
