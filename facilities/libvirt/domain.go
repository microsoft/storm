// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"fmt"

	"libvirt.org/go/libvirtxml"
)

const (
	formatQcow2 = "qcow2"
	formatRaw   = "raw"
)

type diskPlan struct {
	dev    string
	path   string
	format string
	order  uint // libvirt boot order, 0 when not a boot device
}

type cdromPlan struct {
	dev   string
	path  string // empty for an empty bay
	order uint
}

// domainPlan is everything buildDomain needs; it is plain data so the XML
// generation is a pure function.
type domainPlan struct {
	name      string
	arch      string
	cpus      uint
	memoryMiB uint
	mac       string
	network   string
	serialLog string
	tpm       bool
	secure    bool // secure boot firmware requested
	fw        firmware
	disks     []diskPlan
	cdroms    []cdromPlan
}

// newDomainPlan combines a validated spec with the resources created for it.
// vols holds, per spec disk, the path and format of the volume backing it.
func newDomainPlan(name, network, mac string, s Spec, vols []diskPlan, fw firmware) domainPlan {
	p := domainPlan{
		name:      name,
		arch:      s.Arch,
		cpus:      s.CPUs,
		memoryMiB: s.MemoryMiB,
		mac:       mac,
		network:   network,
		serialLog: s.SerialLog,
		tpm:       s.TPM,
		secure:    s.SecureBoot,
		fw:        fw,
	}

	for i, v := range vols {
		v.dev = diskDevice(s.Arch, i)
		v.order = s.Disks[i].BootOrder
		p.disks = append(p.disks, v)
	}
	for i, c := range s.CDROMs {
		p.cdroms = append(p.cdroms, cdromPlan{
			dev:   cdromDevice(s.Arch, len(s.Disks), i),
			path:  c.Path,
			order: c.BootOrder,
		})
	}
	return p
}

func isARM64(arch string) bool { return arch == ArchARM64 }

// target is the libvirt architecture, machine type and virtualisation type a
// Spec.Arch maps to. amd64 guests are hardware accelerated; arm64 guests are
// emulated.
type target struct {
	arch, machine, virtType string
}

func targetFor(arch string) target {
	if isARM64(arch) {
		return target{arch: "aarch64", machine: "virt-6.2", virtType: "qemu"}
	}
	return target{arch: "x86_64", machine: "q35", virtType: "kvm"}
}

// diskDevice returns the guest-visible target name for the i-th disk. amd64
// uses SATA (sdX); arm64 uses virtio (vdX) because SATA with qcow2 does not
// load the EFI firmware reliably on the arm64 virt machine.
func diskDevice(arch string, i int) string {
	prefix := "sd"
	if isARM64(arch) {
		prefix = "vd"
	}
	return fmt.Sprintf("%s%c", prefix, 'a'+i)
}

// cdromDevice returns the target name for the i-th CDROM. CDROMs are always
// SATA (sdX). On amd64 they continue the lettering after the disks so names
// never clash; on arm64 disks are vdX so CDROMs start at sda.
func cdromDevice(arch string, numDisks, i int) string {
	if !isARM64(arch) {
		i += numDisks
	}
	return fmt.Sprintf("sd%c", 'a'+i)
}

func bootOf(order uint) *libvirtxml.DomainDeviceBoot {
	if order == 0 {
		return nil
	}
	return &libvirtxml.DomainDeviceBoot{Order: order}
}

// cdromDisk renders one CDROM drive. It is also used by SetCDROM, so a media
// change keeps the drive's boot order.
func cdromDisk(c cdromPlan) libvirtxml.DomainDisk {
	d := libvirtxml.DomainDisk{
		Device:   "cdrom",
		Driver:   &libvirtxml.DomainDiskDriver{Name: "qemu", Type: formatRaw},
		Target:   &libvirtxml.DomainDiskTarget{Dev: c.dev, Bus: "sata"},
		ReadOnly: &libvirtxml.DomainDiskReadOnly{},
		Boot:     bootOf(c.order),
	}
	if c.path != "" {
		d.Source = &libvirtxml.DomainDiskSource{File: &libvirtxml.DomainDiskSourceFile{File: c.path}}
	}
	return d
}

func (p domainPlan) disksXML() []libvirtxml.DomainDisk {
	out := make([]libvirtxml.DomainDisk, 0, len(p.disks)+len(p.cdroms))
	bus := "sata"
	if isARM64(p.arch) {
		bus = "virtio"
	}
	for _, d := range p.disks {
		out = append(out, libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{Name: "qemu", Type: d.format},
			Source: &libvirtxml.DomainDiskSource{File: &libvirtxml.DomainDiskSourceFile{File: d.path}},
			Target: &libvirtxml.DomainDiskTarget{Dev: d.dev, Bus: bus},
			Boot:   bootOf(d.order),
		})
	}
	for _, c := range p.cdroms {
		out = append(out, cdromDisk(c))
	}
	return out
}

// buildDomain renders the libvirt domain definition for a plan.
func buildDomain(p domainPlan) libvirtxml.Domain {
	arm := isARM64(p.arch)

	tgt := targetFor(p.arch)
	osType := libvirtxml.DomainOSType{Arch: tgt.arch, Machine: tgt.machine, Type: "hvm"}
	domType := tgt.virtType

	dom := libvirtxml.Domain{
		Type:   domType,
		Name:   p.name,
		Memory: &libvirtxml.DomainMemory{Unit: "MiB", Value: p.memoryMiB},
		VCPU:   &libvirtxml.DomainVCPU{Value: p.cpus},
		OS: &libvirtxml.DomainOS{
			Type:   &osType,
			Loader: &libvirtxml.DomainLoader{Path: p.fw.Loader, Type: "pflash", Readonly: "yes"},
			NVRam:  &libvirtxml.DomainNVRam{Template: p.fw.VarsTemplate},
		},
		Features: &libvirtxml.DomainFeatureList{ACPI: &libvirtxml.DomainFeature{}},
		// The host CPU is passed through so the guest sees whichever
		// virtualisation extension the host has (VMX or SVM) and can run
		// nested virtualisation; a fixed model would have to guess.
		CPU:   &libvirtxml.DomainCPU{Mode: "host-passthrough", Check: "none"},
		Clock: &libvirtxml.DomainClock{Offset: "utc"},
		Devices: &libvirtxml.DomainDeviceList{
			Disks: p.disksXML(),
			Interfaces: []libvirtxml.DomainInterface{{
				Model: &libvirtxml.DomainInterfaceModel{Type: "virtio"},
				MAC:   &libvirtxml.DomainInterfaceMAC{Address: p.mac},
				Source: &libvirtxml.DomainInterfaceSource{
					Network: &libvirtxml.DomainInterfaceSourceNetwork{Network: p.network},
				},
			}},
			Serials: []libvirtxml.DomainSerial{{
				Source: &libvirtxml.DomainChardevSource{Pty: &libvirtxml.DomainChardevSourcePty{}},
				Target: &libvirtxml.DomainSerialTarget{Port: new(uint)},
			}},
		},
	}

	if p.secure {
		// Secure boot firmware needs the loader flagged secure and, on the
		// x86 q35 machine, System Management Mode to protect the variables.
		dom.OS.Loader.Secure = "yes"
		if !arm {
			dom.Features.SMM = &libvirtxml.DomainFeatureSMM{State: "on"}
		}
	}

	if p.tpm {
		dom.Devices.TPMs = []libvirtxml.DomainTPM{{
			Model: "tpm-tis",
			Backend: &libvirtxml.DomainTPMBackend{
				Emulator: &libvirtxml.DomainTPMBackendEmulator{Version: "2.0"},
			},
		}}
	}

	if arm {
		// There is no host-passthrough for a TCG guest.
		dom.CPU = &libvirtxml.DomainCPU{
			Match: "exact",
			Check: "none",
			Model: &libvirtxml.DomainCPUModel{Fallback: "forbid", Value: "cortex-a57"},
		}
	}

	if !arm {
		// x86 specifics: APIC, the standard timer stack and no suspend states.
		// There is no display device: the console is the serial port, and not
		// every QEMU build supports SPICE or QXL.
		dom.Features.APIC = &libvirtxml.DomainFeatureAPIC{}
		dom.Features.VMPort = &libvirtxml.DomainFeatureState{State: "off"}
		dom.Clock.Timer = []libvirtxml.DomainTimer{
			{Name: "rtc", TickPolicy: "catchup"},
			{Name: "pit", TickPolicy: "delay"},
			{Name: "hpet", Present: "no"},
		}
		dom.PM = &libvirtxml.DomainPM{
			SuspendToMem:  &libvirtxml.DomainPMPolicy{Enabled: "no"},
			SuspendToDisk: &libvirtxml.DomainPMPolicy{Enabled: "no"},
		}
	}

	return dom
}
