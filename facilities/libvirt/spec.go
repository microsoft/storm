// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.

package libvirt

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Architectures accepted in [Spec.Arch].
const (
	ArchAMD64 = "amd64"
	ArchARM64 = "arm64"
)

// Disk describes one disk of a VM. At least one of SizeGiB and ImagePath must
// be set.
//
// With only SizeGiB, a blank, thin-provisioned qcow2 volume of that capacity is
// created in the run's storage pool.
//
// ImagePath names an image on the machine running this library. It is an
// immutable source: its contents are copied (uploaded) into a new volume owned
// by the run, and the source file is neither modified nor attached to the VM.
// The format is detected from the file header: a qcow2 image becomes a qcow2
// volume, anything else is treated as a raw image and becomes a raw volume.
// The volume has the image's virtual size, so the copy of a large image takes
// as long as the upload.
//
// With both, the image is copied and the volume is then enlarged to SizeGiB.
// SizeGiB must not be smaller than the image's virtual size (shrinking is
// refused). Enlarging only extends the virtual disk; growing the guest's
// partitions and filesystems is up to the guest.
//
// BootOrder is described on [Spec].
type Disk struct {
	SizeGiB   uint
	ImagePath string
	BootOrder uint
}

// CDROM describes one optical drive. Path is the absolute path, on the
// libvirt host, of the ISO to insert; it must be readable by the hypervisor
// and allowed by the host's confinement policy. It may be empty to create an
// empty bay that [VM.SetCDROM] can fill later. BootOrder is described on
// [Spec].
type CDROM struct {
	Path      string
	BootOrder uint
}

// Spec describes a VM to provision.
//
// Boot order: BootOrder on a Disk or CDROM is one global ranking across both
// kinds of device. 0 means the device is not a boot device; a positive value
// is its libvirt boot order, and the firmware tries devices in ascending order
// (1 first). Positive values must be unique across all Disks and CDROMs; they
// need not be contiguous. If no device has a BootOrder, the firmware default
// applies.
//
// The machine uses UEFI firmware (OVMF for amd64, AAVMF for arm64). amd64
// guests are KVM accelerated q35 machines with SATA disks; arm64 guests use
// the TCG emulated "virt" machine with virtio disks. Provision checks the
// libvirt host's capabilities and fails if SecureBoot or TPM, or the
// architecture itself, cannot be provided; it never falls back to a weaker
// configuration.
type Spec struct {
	// Name identifies the VM within the run: lowercase letters, digits and
	// single hyphens. The libvirt domain is named
	// "<namespace>_<run ID>_<Name>".
	Name string

	CPUs      uint
	MemoryMiB uint

	// Arch is ArchAMD64 (default when empty) or ArchARM64.
	Arch string

	Disks  []Disk
	CDROMs []CDROM

	// SecureBoot selects the secure boot capable firmware with the
	// Microsoft keys enrolled.
	SecureBoot bool

	// TPM attaches an emulated TPM 2.0 device (requires swtpm on the host).
	TPM bool

	// SerialLog is a path on the machine running this library. When set, the
	// guest's first serial port is captured into that file from [VM.Start]
	// on, over the libvirt connection, so it works with remote libvirt URIs.
	// The file is created (mode 0644, owned by the calling user) if missing and
	// appended to, across guest reboots and VM restarts; it is never truncated
	// or removed by this library. Capture ends when the VM is shut off, the
	// client is closed or [Client.Destroy] runs.
	SerialLog string
}

// maxDrives is the number of distinct target device names (sda..sdz) we can
// hand out across disks and CDROMs.
const maxDrives = 26

func (s Spec) withDefaults() Spec {
	if s.Arch == "" {
		s.Arch = ArchAMD64
	}
	return s
}

// validate checks the spec without touching the filesystem or libvirt. It
// expects defaults to have been applied.
func (s Spec) validate() error {
	var errs []error
	if err := validateName("VM name", s.Name); err != nil {
		errs = append(errs, err)
	}
	if s.CPUs == 0 {
		errs = append(errs, errors.New("CPUs must be > 0"))
	}
	if s.MemoryMiB == 0 {
		errs = append(errs, errors.New("MemoryMiB must be > 0"))
	}
	if s.Arch != ArchAMD64 && s.Arch != ArchARM64 {
		errs = append(errs, fmt.Errorf("unsupported architecture %q (want %q or %q)", s.Arch, ArchAMD64, ArchARM64))
	}
	if len(s.Disks) == 0 && len(s.CDROMs) == 0 {
		errs = append(errs, errors.New("at least one disk or CDROM is required"))
	}
	if len(s.Disks)+len(s.CDROMs) > maxDrives {
		errs = append(errs, fmt.Errorf("too many drives: %d disks + %d CDROMs exceeds %d", len(s.Disks), len(s.CDROMs), maxDrives))
	}
	orders := map[uint]string{}
	checkOrder := func(order uint, what string) {
		if order == 0 {
			return
		}
		if prev, dup := orders[order]; dup {
			errs = append(errs, fmt.Errorf("%s: boot order %d is already used by %s", what, order, prev))
			return
		}
		orders[order] = what
	}
	for i, d := range s.Disks {
		what := fmt.Sprintf("disk %d", i)
		if d.SizeGiB == 0 && d.ImagePath == "" {
			errs = append(errs, fmt.Errorf("%s: one of SizeGiB or ImagePath is required", what))
		}
		if d.ImagePath != "" && !filepath.IsAbs(d.ImagePath) {
			errs = append(errs, fmt.Errorf("%s: ImagePath %q must be absolute", what, d.ImagePath))
		}
		checkOrder(d.BootOrder, what)
	}
	for i, c := range s.CDROMs {
		what := fmt.Sprintf("CDROM %d", i)
		if c.Path != "" && !filepath.IsAbs(c.Path) {
			errs = append(errs, fmt.Errorf("%s: path %q must be absolute", what, c.Path))
		}
		checkOrder(c.BootOrder, what)
	}
	if s.SerialLog != "" && !filepath.IsAbs(s.SerialLog) {
		errs = append(errs, fmt.Errorf("SerialLog %q must be an absolute path", s.SerialLog))
	}
	return errors.Join(errs...)
}
