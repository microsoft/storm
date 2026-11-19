// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.

package libvirt

import (
	"fmt"
	"slices"

	golibvirt "github.com/digitalocean/go-libvirt"
	"libvirt.org/go/libvirtxml"
)

// requirements are the host capabilities a Spec asks for.
type requirements struct {
	secureBoot bool
	tpm        bool
}

// checkDomainCaps verifies, from a libvirt domain capabilities document, that
// the host can provide what was requested. Unsupported requests are errors:
// nothing is silently downgraded.
func checkDomainCaps(xml string, t target, req requirements) error {
	var caps libvirtxml.DomainCaps
	if err := caps.Unmarshal(xml); err != nil {
		return fmt.Errorf("parse domain capabilities: %w", err)
	}

	where := fmt.Sprintf("%s/%s/%s", t.arch, t.machine, t.virtType)
	if caps.OS == nil || caps.OS.Supported != "yes" || caps.OS.Loader == nil || caps.OS.Loader.Supported != "yes" {
		return fmt.Errorf("host does not support UEFI firmware loaders for %s", where)
	}
	if !enumHas(caps.OS.Loader.Enums, "type", "pflash") {
		return fmt.Errorf("host does not support pflash firmware for %s", where)
	}
	if req.secureBoot && !enumHas(caps.OS.Loader.Enums, "secure", "yes") {
		return fmt.Errorf("host does not support secure boot for %s", where)
	}
	if req.tpm {
		tpm := (*libvirtxml.DomainCapsDevice)(nil)
		if caps.Devices != nil {
			tpm = caps.Devices.TPM
		}
		if tpm == nil || tpm.Supported != "yes" ||
			!enumHas(tpm.Enums, "backendModel", "emulator") ||
			!enumHas(tpm.Enums, "backendVersion", "2.0") ||
			!enumHas(tpm.Enums, "model", "tpm-tis") {
			return fmt.Errorf("host does not support an emulated TPM 2.0 for %s", where)
		}
	}
	return nil
}

func enumHas(enums []libvirtxml.DomainCapsEnum, name, value string) bool {
	for _, e := range enums {
		if e.Name == name {
			return slices.Contains(e.Values, value)
		}
	}
	return false
}

// checkHost queries libvirt for the capabilities of the target and checks the
// request against them.
func (c *Client) checkHost(t target, req requirements) error {
	xml, err := c.lv.ConnectGetDomainCapabilities(golibvirt.OptString{}, golibvirt.OptString{t.arch}, golibvirt.OptString{t.machine}, golibvirt.OptString{t.virtType}, 0)
	if err != nil {
		return fmt.Errorf("query host capabilities for %s/%s/%s (is the emulator installed and, for amd64, KVM available?): %w", t.arch, t.machine, t.virtType, err)
	}
	return checkDomainCaps(xml, t, req)
}
