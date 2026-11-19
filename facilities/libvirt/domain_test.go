package libvirt

import (
	"testing"

	"libvirt.org/go/libvirtxml"
)

func testFirmware() firmware {
	return firmware{Loader: "/fw/CODE.fd", VarsTemplate: "/fw/VARS.fd"}
}

func renderDomain(t *testing.T, p domainPlan) libvirtxml.Domain {
	t.Helper()
	dom := buildDomain(p)
	xml, err := dom.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back libvirtxml.Domain
	if err := back.Unmarshal(xml); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, xml)
	}
	return back
}

func diskByDev(t *testing.T, d libvirtxml.Domain, dev string) libvirtxml.DomainDisk {
	t.Helper()
	for _, disk := range d.Devices.Disks {
		if disk.Target.Dev == dev {
			return disk
		}
	}
	t.Fatalf("no disk with target %q", dev)
	return libvirtxml.DomainDisk{}
}

func sourcePath(d libvirtxml.DomainDisk) string {
	if d.Source == nil || d.Source.File == nil {
		return ""
	}
	return d.Source.File.File
}

func bootOrder(d libvirtxml.DomainDisk) uint {
	if d.Boot == nil {
		return 0
	}
	return d.Boot.Order
}

func TestBuildDomainDisksCDROMsAndBootOrder(t *testing.T) {
	spec := Spec{
		Name: "vm0", CPUs: 2, MemoryMiB: 2048, Arch: ArchAMD64,
		Disks: []Disk{
			{SizeGiB: 8},                // not bootable
			{SizeGiB: 16, BootOrder: 3}, // third
		},
		CDROMs: []CDROM{
			{Path: "/isos/install.iso", BootOrder: 1}, // first
			{}, // empty bay, not bootable
		},
		SerialLog: "/logs/vm0-serial.log",
	}
	vols := []diskPlan{
		{path: "/pool/a.qcow2", format: formatQcow2},
		{path: "/pool/b.img", format: formatRaw},
	}
	plan := newDomainPlan("ns_vm0", "ns_net", "52:54:00:aa:bb:cc", spec, vols, testFirmware())
	dom := renderDomain(t, plan)

	if dom.Name != "ns_vm0" || dom.Type != "kvm" {
		t.Errorf("name/type = %q/%q", dom.Name, dom.Type)
	}
	if dom.Memory.Unit != "MiB" || dom.Memory.Value != 2048 || dom.VCPU.Value != 2 {
		t.Errorf("memory/vcpu = %+v / %+v", dom.Memory, dom.VCPU)
	}
	if len(dom.Devices.Disks) != 4 {
		t.Fatalf("got %d disks, want 4", len(dom.Devices.Disks))
	}

	d0 := diskByDev(t, dom, "sda")
	d1 := diskByDev(t, dom, "sdb")
	c0 := diskByDev(t, dom, "sdc")
	c1 := diskByDev(t, dom, "sdd")

	if d0.Device != "disk" || d0.Driver.Type != formatQcow2 || d0.Source.File.File != "/pool/a.qcow2" || d0.Target.Bus != "sata" {
		t.Errorf("disk 0 unexpected: %+v", d0)
	}
	if d1.Driver.Type != formatRaw || d1.Source.File.File != "/pool/b.img" {
		t.Errorf("disk 1 unexpected: %+v", d1)
	}
	if c0.Device != "cdrom" || c0.Source == nil || c0.Source.File.File != "/isos/install.iso" || c0.ReadOnly == nil {
		t.Errorf("cdrom 0 unexpected: %+v", c0)
	}
	if c1.Device != "cdrom" || sourcePath(c1) != "" {
		t.Errorf("cdrom 1 should be an empty bay: %+v", c1)
	}

	// BootOrder is one global ranking across disks and CDROMs.
	for dev, want := range map[string]uint{"sda": 0, "sdb": 3, "sdc": 1, "sdd": 0} {
		if got := bootOrder(diskByDev(t, dom, dev)); got != want {
			t.Errorf("boot order of %s = %d, want %d", dev, got, want)
		}
	}

	if len(dom.Devices.Serials) != 1 {
		t.Fatalf("got %d serials, want 1", len(dom.Devices.Serials))
	}
	ser := dom.Devices.Serials[0]
	// The console is captured in-process, so libvirt must not write a log
	// file of its own (it would be root-owned and local to the host).
	if ser.Log != nil {
		t.Errorf("serial device has a libvirt-side log: %+v", ser.Log)
	}
	if ser.Target == nil || ser.Target.Port == nil || *ser.Target.Port != 0 {
		t.Errorf("serial target = %+v", ser.Target)
	}
	if ser.Source == nil || ser.Source.Pty == nil {
		t.Errorf("serial source is not a pty: %+v", ser.Source)
	}

	if dom.OS.Loader.Path != "/fw/CODE.fd" || dom.OS.Loader.Type != "pflash" {
		t.Errorf("loader = %+v", dom.OS.Loader)
	}
	if dom.OS.NVRam == nil || dom.OS.NVRam.Template != "/fw/VARS.fd" || dom.OS.NVRam.NVRam != "" {
		t.Errorf("nvram = %+v", dom.OS.NVRam)
	}
	if len(dom.Devices.TPMs) != 0 {
		t.Errorf("unexpected TPM: %+v", dom.Devices.TPMs)
	}

	nic := dom.Devices.Interfaces[0]
	if nic.MAC.Address != "52:54:00:aa:bb:cc" || nic.Source.Network.Network != "ns_net" {
		t.Errorf("interface = %+v", nic)
	}
}

func TestBuildDomainBootOrderUnsetWhenNothingBootable(t *testing.T) {
	spec := Spec{Name: "v", CPUs: 1, MemoryMiB: 512, Arch: ArchAMD64, Disks: []Disk{{SizeGiB: 1}}}
	plan := newDomainPlan("ns_v", "ns_net", "52:54:00:00:00:01", spec, []diskPlan{{path: "/p", format: formatQcow2}}, testFirmware())
	dom := renderDomain(t, plan)
	for _, d := range dom.Devices.Disks {
		if d.Boot != nil {
			t.Errorf("no disk is bootable but %s has boot order %d", d.Target.Dev, d.Boot.Order)
		}
	}
}

func TestBuildDomainSecureBootAndTPM(t *testing.T) {
	spec := Spec{Name: "v", CPUs: 1, MemoryMiB: 512, Arch: ArchAMD64, Disks: []Disk{{SizeGiB: 1}}, SecureBoot: true, TPM: true}
	fw, err := findFirmware(ArchAMD64, true, func(p string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	plan := newDomainPlan("ns_v", "ns_net", "52:54:00:00:00:01", spec, []diskPlan{{path: "/p", format: formatQcow2}}, fw)
	dom := renderDomain(t, plan)

	if dom.OS.Loader.Path != "/usr/share/OVMF/OVMF_CODE_4M.ms.fd" {
		t.Errorf("secure boot loader = %q", dom.OS.Loader.Path)
	}
	if dom.OS.NVRam.Template != "/usr/share/OVMF/OVMF_VARS_4M.ms.fd" {
		t.Errorf("secure boot vars template = %q", dom.OS.NVRam.Template)
	}
	if dom.OS.Loader.Secure != "yes" {
		t.Errorf("loader secure = %q, want yes", dom.OS.Loader.Secure)
	}
	if dom.Features.SMM == nil || dom.Features.SMM.State != "on" {
		t.Errorf("SMM = %+v, want on", dom.Features.SMM)
	}
	if len(dom.Devices.TPMs) != 1 {
		t.Fatalf("got %d TPMs, want 1", len(dom.Devices.TPMs))
	}
	tpm := dom.Devices.TPMs[0]
	if tpm.Backend == nil || tpm.Backend.Emulator == nil || tpm.Backend.Emulator.Version != "2.0" {
		t.Errorf("TPM backend = %+v", tpm.Backend)
	}
}

func TestBuildDomainWithoutSecureBootHasNoSMM(t *testing.T) {
	spec := Spec{Name: "v", CPUs: 1, MemoryMiB: 512, Arch: ArchAMD64, Disks: []Disk{{SizeGiB: 1}}}
	plan := newDomainPlan("ns_v", "ns_net", "52:54:00:00:00:01", spec, []diskPlan{{path: "/p", format: formatQcow2}}, testFirmware())
	dom := renderDomain(t, plan)
	if dom.OS.Loader.Secure != "" || dom.Features.SMM != nil {
		t.Errorf("secure = %q, SMM = %+v; want neither", dom.OS.Loader.Secure, dom.Features.SMM)
	}
}

func TestBuildDomainARM64(t *testing.T) {
	spec := Spec{
		Name: "v", CPUs: 1, MemoryMiB: 512, Arch: ArchARM64,
		Disks:  []Disk{{SizeGiB: 1, BootOrder: 1}},
		CDROMs: []CDROM{{}},
	}
	plan := newDomainPlan("ns_v", "ns_net", "52:54:00:00:00:01", spec, []diskPlan{{path: "/p", format: formatQcow2}}, testFirmware())
	dom := renderDomain(t, plan)

	if dom.Type != "qemu" || dom.OS.Type.Arch != "aarch64" {
		t.Errorf("type/arch = %q/%q", dom.Type, dom.OS.Type.Arch)
	}
	if d := diskByDev(t, dom, "vda"); d.Target.Bus != "virtio" || bootOrder(d) != 1 {
		t.Errorf("arm64 disk = %+v", d)
	}
	if c := diskByDev(t, dom, "sda"); c.Device != "cdrom" {
		t.Errorf("arm64 cdrom = %+v", c)
	}
}

func TestCPUModel(t *testing.T) {
	for arch, want := range map[string]string{ArchAMD64: "host-passthrough", ArchARM64: ""} {
		spec := Spec{Name: "v", CPUs: 1, MemoryMiB: 512, Arch: arch, Disks: []Disk{{SizeGiB: 1, BootOrder: 1}}}
		plan := newDomainPlan("ns_v", "ns_net", "52:54:00:00:00:01", spec, []diskPlan{{path: "/p", format: formatQcow2}}, testFirmware())
		cpu := renderDomain(t, plan).CPU
		if cpu.Mode != want || (want == "" && cpu.Model == nil) {
			t.Errorf("%s: cpu = %+v", arch, cpu)
		}
	}
}

func TestNoDisplayDevices(t *testing.T) {
	for _, arch := range []string{ArchAMD64, ArchARM64} {
		spec := Spec{Name: "v", CPUs: 1, MemoryMiB: 512, Arch: arch, Disks: []Disk{{SizeGiB: 1, BootOrder: 1}}}
		plan := newDomainPlan("ns_v", "ns_net", "52:54:00:00:00:01", spec, []diskPlan{{path: "/p", format: formatQcow2}}, testFirmware())
		d := renderDomain(t, plan).Devices
		if len(d.Graphics) != 0 || len(d.Videos) != 0 || len(d.Channels) != 0 {
			t.Errorf("%s: unexpected display devices: %+v %+v %+v", arch, d.Graphics, d.Videos, d.Channels)
		}
	}
}

func TestCDROMDiskKeepsBootOrderOnMediaChange(t *testing.T) {
	c := cdromPlan{dev: "sdc", order: 2}
	empty := cdromDisk(c)
	if sourcePath(empty) != "" || bootOrder(empty) != 2 {
		t.Errorf("empty bay = %+v", empty)
	}
	c.path = "/isos/new.iso"
	filled := cdromDisk(c)
	if filled.Source == nil || filled.Source.File.File != "/isos/new.iso" || bootOrder(filled) != 2 {
		t.Errorf("filled bay = %+v", filled)
	}
	xml, err := filled.Marshal()
	if err != nil || xml == "" {
		t.Errorf("Marshal: %q, %v", xml, err)
	}
}

func TestFindFirmware(t *testing.T) {
	has := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, x := range paths {
				if x == p {
					return true
				}
			}
			return false
		}
	}

	fw, err := findFirmware(ArchAMD64, false, has("/usr/share/OVMF/OVMF_CODE_4M.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd"))
	if err != nil || fw.Loader != "/usr/share/OVMF/OVMF_CODE_4M.fd" || fw.VarsTemplate != "/usr/share/OVMF/OVMF_VARS_4M.fd" {
		t.Errorf("amd64 = %+v, %v", fw, err)
	}
	fw, err = findFirmware(ArchARM64, true, has("/usr/share/AAVMF/AAVMF_CODE.ms.fd", "/usr/share/AAVMF/AAVMF_VARS.ms.fd"))
	if err != nil || fw.Loader != "/usr/share/AAVMF/AAVMF_CODE.ms.fd" {
		t.Errorf("arm64 secure boot = %+v, %v", fw, err)
	}
	fw, err = findFirmware(ArchAMD64, false, has("/usr/share/edk2/ovmf/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_VARS.fd"))
	if err != nil || fw.Loader != "/usr/share/edk2/ovmf/OVMF_CODE.fd" {
		t.Errorf("edk2 fallback = %+v, %v", fw, err)
	}
	// Code present but vars template missing: not usable.
	if _, err := findFirmware(ArchAMD64, false, has("/usr/share/OVMF/OVMF_CODE_4M.fd")); err == nil {
		t.Error("expected error when vars template is missing")
	}
	if _, err := findFirmware("riscv", false, has()); err == nil {
		t.Error("expected error for unknown arch")
	}
}
