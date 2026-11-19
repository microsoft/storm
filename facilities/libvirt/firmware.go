// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"fmt"
	"os"
)

// firmware holds the UEFI code image and the variable-store template for a
// VM. libvirt copies the template into a per-domain NVRAM file, and removes
// that file again when the domain is undefined with the NVRAM flag.
type firmware struct {
	Loader       string
	VarsTemplate string
}

type firmwareCandidate struct {
	loader, vars string
}

// firmwareCandidates lists, per architecture and secure boot setting, the
// firmware code/vars pairs we know about in order of preference. The first
// pair whose two files both exist wins. The Debian/Ubuntu paths come first;
// the edk2 paths are used by Fedora-family distributions.
var firmwareCandidates = map[string]map[bool][]firmwareCandidate{
	ArchAMD64: {
		false: {
			{"/usr/share/OVMF/OVMF_CODE_4M.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd"},
			{"/usr/share/edk2/ovmf/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_VARS.fd"},
		},
		true: {
			{"/usr/share/OVMF/OVMF_CODE_4M.ms.fd", "/usr/share/OVMF/OVMF_VARS_4M.ms.fd"},
			{"/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd", "/usr/share/edk2/ovmf/OVMF_VARS.secboot.fd"},
		},
	},
	ArchARM64: {
		false: {
			{"/usr/share/AAVMF/AAVMF_CODE.fd", "/usr/share/AAVMF/AAVMF_VARS.fd"},
		},
		true: {
			{"/usr/share/AAVMF/AAVMF_CODE.ms.fd", "/usr/share/AAVMF/AAVMF_VARS.ms.fd"},
		},
	},
}

// findFirmware picks the first installed firmware pair for the architecture.
// exists is injectable so the lookup can be tested without the firmware
// packages installed.
func findFirmware(arch string, secureBoot bool, exists func(string) bool) (firmware, error) {
	cands := firmwareCandidates[arch][secureBoot]
	if len(cands) == 0 {
		return firmware{}, fmt.Errorf("no firmware known for architecture %q (secure boot: %t)", arch, secureBoot)
	}
	for _, c := range cands {
		if exists(c.loader) && exists(c.vars) {
			return firmware{Loader: c.loader, VarsTemplate: c.vars}, nil
		}
	}
	return firmware{}, fmt.Errorf("UEFI firmware for %s (secure boot: %t) not found; install the OVMF/AAVMF package (looked for %s)",
		arch, secureBoot, describeCandidates(cands))
}

func describeCandidates(cands []firmwareCandidate) string {
	s := ""
	for i, c := range cands {
		if i > 0 {
			s += ", "
		}
		s += c.loader
	}
	return s
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
