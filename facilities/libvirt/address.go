// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

// ErrNoAddress is returned (wrapped) by [VM.Address] while the VM has no DHCP
// lease yet. Callers are expected to poll.
var ErrNoAddress = errors.New("no DHCP lease for VM")

// leaseQuery returns the IPv4 addresses the DHCP server currently has leased
// to the VM. It is a field of VM so tests can replace it.
type leaseQuery func(ctx context.Context) ([]netip.Addr, error)

// stabilizeConfig bounds the wait for duplicate leases to settle.
type stabilizeConfig struct {
	retries  int
	interval time.Duration
}

var defaultStabilize = stabilizeConfig{retries: 5, interval: 2 * time.Second}

// ipv4Leases extracts the IPv4 addresses for the interface with the given MAC
// from a libvirt DomainInterfaceAddresses (lease source) reply. Interfaces
// that report a different hardware address are ignored.
func ipv4Leases(ifaces []golibvirt.DomainInterface, mac string) []netip.Addr {
	var out []netip.Addr
	for _, ifc := range ifaces {
		if len(ifc.Hwaddr) > 0 && !strings.EqualFold(ifc.Hwaddr[0], mac) {
			continue
		}
		for _, a := range ifc.Addrs {
			if a.Type != int32(golibvirt.IPAddrTypeIpv4) {
				continue
			}
			addr, err := netip.ParseAddr(a.Addr)
			if err != nil || !addr.Is4() {
				continue
			}
			out = append(out, addr)
		}
	}
	return out
}

// resolveAddress implements the lease lookup with the duplicate-lease
// workaround: when a guest sends duplicate DHCPDISCOVER packets at boot, the
// dnsmasq lease file can briefly hold several entries for one MAC, so more
// than one address is reported. In that case the query is repeated (bounded by
// cfg) until it settles on a single address. If it never does, the first
// address of the latest answer is returned. A query that fails or returns
// nothing yields an error wrapping [ErrNoAddress] so that the caller can poll.
func resolveAddress(ctx context.Context, query leaseQuery, cfg stabilizeConfig) (netip.Addr, error) {
	ips, err := query(ctx)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("query leases: %w", err)
	}
	switch len(ips) {
	case 0:
		return netip.Addr{}, ErrNoAddress
	case 1:
		return ips[0], nil
	}

	latest := ips
	for range cfg.retries {
		if err := sleepCtx(ctx, cfg.interval); err != nil {
			return netip.Addr{}, err
		}
		retry, err := query(ctx)
		if err != nil || len(retry) == 0 {
			continue
		}
		latest = retry
		if len(retry) == 1 {
			break
		}
	}
	return latest[0], nil
}

// sleepCtx waits for d or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
