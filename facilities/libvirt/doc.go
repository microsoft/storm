// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.

// Package libvirt provisions and drives throw-away virtual machines on a
// libvirt/QEMU host.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).
//
// # Ownership
//
// A [Client] owns one run. Every libvirt object it creates (domains, the NAT
// network, the storage pool and its volumes) is named
// "<namespace>_<run ID>_<something>". The run ID is random unless the caller
// supplies one, so two clients never share objects, not even with the same
// namespace, and [Client.Destroy] only removes objects with its own prefix. The
// separator "_" is not allowed inside namespaces, run IDs and VM names, so a
// prefix can never match the objects of a different namespace or run.
//
// The ownership information lives in the names, which libvirt persists, so a
// process that crashed leaves objects that a later process can find with
// [Client.Runs] and remove by connecting with [Options.RunID] set. Taking over
// an existing run is always explicit; a client with a fresh run ID refuses to
// reuse a network or pool that already exists.
//
// Connection lifetime is separate from ownership: [Client.Close] only
// disconnects.
//
// # Networking
//
// Each run gets its own NAT network with a DHCP range, created once on the
// first [Client.Provision]. The /24 subnet inside 192.168.0.0/16 is derived
// from a hash of namespace and run ID, skipping subnets already used by
// existing libvirt networks and host interfaces. 192.168.0.0/24,
// 192.168.1.0/24 and the libvirt default 192.168.122.0/24 are never used.
// Because processes can race, the new network is checked after it has started
// against all other libvirt networks; on a conflict it is removed and another
// subnet is tried.
//
// # Typical use
//
//	c, err := libvirt.Connect(ctx, "", libvirt.Options{Namespace: "mytest"})
//	if err != nil { ... }
//	defer c.Close()
//	defer func() {
//		// Cleanup must outlive a cancelled run: keep the values of ctx but
//		// not its cancellation, and give it its own budget.
//		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
//		defer cancel()
//		_ = c.Destroy(dctx)
//	}()
//	vm, err := c.Provision(ctx, libvirt.Spec{Name: "vm0", CPUs: 2, MemoryMiB: 2048,
//		Disks: []libvirt.Disk{{SizeGiB: 20, BootOrder: 1}}})
//	...
//	err = vm.Start(ctx)
//
// # Contexts
//
// Every blocking method takes a context and uses no deadline of its own. The
// libvirt RPC client used underneath is not context aware, so a single RPC is
// never interrupted; the context is checked between calls and while polling.
// Long uploads ([Disk.ImagePath]) are aborted at the next read once the context
// ends. Provision's cleanup after a failure is the one exception: it runs with
// a short context of its own so that it still happens after a cancellation.
//
// # Host assumptions
//
// UEFI firmware files are looked up on the local filesystem, and local host
// interfaces are consulted when choosing a subnet, so the libvirt daemon is
// expected to run on the machine running this library; the default
// "qemu:///system" does. [CDROM.Path] is a path on the libvirt host.
package libvirt
