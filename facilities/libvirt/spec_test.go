package libvirt

import (
	"strings"
	"testing"
)

func validSpec() Spec {
	return Spec{
		Name:      "vm0",
		CPUs:      2,
		MemoryMiB: 1024,
		Disks:     []Disk{{SizeGiB: 10, BootOrder: 1}},
		CDROMs:    []CDROM{{}},
	}.withDefaults()
}

func TestSpecDefaults(t *testing.T) {
	if got := (Spec{}).withDefaults().Arch; got != ArchAMD64 {
		t.Errorf("default arch = %q, want %q", got, ArchAMD64)
	}
	if got := (Spec{Arch: ArchARM64}).withDefaults().Arch; got != ArchARM64 {
		t.Errorf("arch = %q, want %q", got, ArchARM64)
	}
}

func TestSpecValidate(t *testing.T) {
	if err := validSpec().validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Spec)
		want   string
	}{
		{"bad name", func(s *Spec) { s.Name = "Bad_Name" }, "VM name"},
		{"no cpus", func(s *Spec) { s.CPUs = 0 }, "CPUs"},
		{"no memory", func(s *Spec) { s.MemoryMiB = 0 }, "MemoryMiB"},
		{"bad arch", func(s *Spec) { s.Arch = "riscv" }, "architecture"},
		{"no drives", func(s *Spec) { s.Disks, s.CDROMs = nil, nil }, "at least one"},
		{"disk without source", func(s *Spec) { s.Disks = []Disk{{}} }, "one of SizeGiB or ImagePath"},
		{"relative image", func(s *Spec) { s.Disks = []Disk{{ImagePath: "x.qcow2"}} }, "ImagePath"},
		{"duplicate boot order", func(s *Spec) {
			s.Disks = []Disk{{SizeGiB: 1, BootOrder: 2}}
			s.CDROMs = []CDROM{{BootOrder: 2}}
		}, "boot order 2 is already used by disk 0"},
		{"duplicate boot order across disks", func(s *Spec) {
			s.Disks = []Disk{{SizeGiB: 1, BootOrder: 1}, {SizeGiB: 1, BootOrder: 1}}
		}, "already used"},
		{"relative cdrom", func(s *Spec) { s.CDROMs = []CDROM{{Path: "rel.iso"}} }, "absolute"},
		{"relative serial log", func(s *Spec) { s.SerialLog = "serial.log" }, "SerialLog"},
		{"too many drives", func(s *Spec) {
			s.Disks = make([]Disk, maxDrives)
			for i := range s.Disks {
				s.Disks[i].SizeGiB = 1
			}
		}, "too many drives"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec()
			tt.mutate(&s)
			err := s.validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate() = %v, want error containing %q", err, tt.want)
			}
		})
	}

	ok := validSpec()
	ok.Disks = []Disk{{ImagePath: "/images/os.qcow2"}}
	ok.CDROMs = []CDROM{{Path: "/isos/a.iso"}, {}}
	ok.SerialLog = "/var/log/serial.log"
	if err := ok.validate(); err != nil {
		t.Errorf("valid spec with image/iso/serial rejected: %v", err)
	}

	// SizeGiB together with ImagePath means "copy, then enlarge".
	grow := validSpec()
	grow.Disks = []Disk{{ImagePath: "/images/os.qcow2", SizeGiB: 40, BootOrder: 1}}
	grow.CDROMs = []CDROM{{BootOrder: 2}, {BootOrder: 5}, {}}
	if err := grow.validate(); err != nil {
		t.Errorf("grow spec / non-contiguous boot orders rejected: %v", err)
	}
}
