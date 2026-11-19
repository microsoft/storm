// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"

	"libvirt.org/go/libvirtxml"
)

// Subnets are /24s of the form 192.168.N.0/24. N ranges over
// [subnetFirst, subnetLast]; subnetReserved are never handed out because they
// are common home/lab LAN or libvirt default ranges.
const (
	subnetFirst = 2
	subnetLast  = 255
)

var subnetReserved = map[int]bool{122: true}

// chooseSubnet deterministically picks a /24 for a run. The starting point is
// derived from a hash of key (namespace and run ID), so different runs spread
// over the range and the choice is repeatable; if that subnet overlaps
// anything in used it probes the following ones, wrapping around.
func chooseSubnet(key string, used []netip.Prefix) (netip.Prefix, error) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	span := subnetLast - subnetFirst + 1
	start := int(h.Sum32() % uint32(span))

	for i := range span {
		n := subnetFirst + (start+i)%span
		if subnetReserved[n] {
			continue
		}
		cand := netip.PrefixFrom(netip.AddrFrom4([4]byte{192, 168, byte(n), 0}), 24)
		if !overlapsAny(cand, used) {
			return cand, nil
		}
	}
	return netip.Prefix{}, fmt.Errorf("no free 192.168.N.0/24 subnet left for %q", key)
}

func overlapsAny(p netip.Prefix, used []netip.Prefix) bool {
	for _, u := range used {
		if u.Overlaps(p) {
			return true
		}
	}
	return false
}

// networkXML describes the per-namespace NAT network: the gateway is .1 and
// DHCP hands out .2 through .254.
func networkXML(name string, subnet netip.Prefix) (string, error) {
	base := subnet.Masked().Addr().As4()
	addr := func(host byte) string {
		a := base
		a[3] = host
		return netip.AddrFrom4(a).String()
	}
	mask := net.CIDRMask(subnet.Bits(), 32)

	n := libvirtxml.Network{
		Name:    name,
		Forward: &libvirtxml.NetworkForward{Mode: "nat"},
		IPs: []libvirtxml.NetworkIP{{
			Address: addr(1),
			Netmask: net.IP(mask).String(),
			DHCP: &libvirtxml.NetworkDHCP{
				Ranges: []libvirtxml.NetworkDHCPRange{{Start: addr(2), End: addr(254)}},
			},
		}},
	}
	return n.Marshal()
}

// networkPrefixes extracts the IPv4 subnets configured on a libvirt network
// definition.
func networkPrefixes(xml string) ([]netip.Prefix, error) {
	var n libvirtxml.Network
	if err := n.Unmarshal(xml); err != nil {
		return nil, fmt.Errorf("parse network XML: %w", err)
	}
	var out []netip.Prefix
	for _, ip := range n.IPs {
		addr, err := netip.ParseAddr(ip.Address)
		if err != nil || !addr.Is4() {
			continue
		}
		bits := int(ip.Prefix)
		if ip.Netmask != "" {
			m, err := netip.ParseAddr(ip.Netmask)
			if err != nil || !m.Is4() {
				continue
			}
			ones, _ := net.IPMask(m.AsSlice()).Size()
			bits = ones
		}
		if bits == 0 {
			bits = 24
		}
		p, err := addr.Prefix(bits)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// hostPrefixes returns the IPv4 subnets of the local host's interfaces.
func hostPrefixes() []netip.Prefix {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipn.IP)
		if !ok {
			continue
		}
		ip = ip.Unmap()
		if !ip.Is4() {
			continue
		}
		ones, _ := ipn.Mask.Size()
		if p, err := ip.Prefix(ones); err == nil {
			out = append(out, p)
		}
	}
	return out
}
