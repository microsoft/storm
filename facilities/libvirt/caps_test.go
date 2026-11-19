package libvirt

import (
	"strings"
	"testing"
)

const capsXML = `<domainCapabilities>
  <path>/usr/bin/qemu-system-x86_64</path>
  <domain>kvm</domain>
  <machine>pc-q35</machine>
  <arch>x86_64</arch>
  <os supported='yes'>
    <loader supported='yes'>
      <enum name='type'><value>rom</value><value>pflash</value></enum>
      <enum name='secure'>%SECURE%</enum>
    </loader>
  </os>
  <devices>
    %TPM%
  </devices>
</domainCapabilities>`

const tpmXML = `<tpm supported='yes'>
  <enum name='model'><value>tpm-tis</value></enum>
  <enum name='backendModel'><value>emulator</value></enum>
  <enum name='backendVersion'><value>%VER%</value></enum>
</tpm>`

func caps(secure, tpm string) string {
	r := strings.NewReplacer("%SECURE%", secure, "%TPM%", tpm)
	return r.Replace(capsXML)
}

func TestCheckDomainCaps(t *testing.T) {
	tgt := targetFor(ArchAMD64)
	secureYes := "<value>yes</value><value>no</value>"
	secureNo := "<value>no</value>"
	tpm20 := strings.ReplaceAll(tpmXML, "%VER%", "2.0")
	tpm12 := strings.ReplaceAll(tpmXML, "%VER%", "1.2")

	tests := []struct {
		name    string
		xml     string
		req     requirements
		wantErr string
	}{
		{"nothing special", caps(secureNo, ""), requirements{}, ""},
		{"secure boot ok", caps(secureYes, ""), requirements{secureBoot: true}, ""},
		{"secure boot unsupported", caps(secureNo, ""), requirements{secureBoot: true}, "secure boot"},
		{"tpm ok", caps(secureNo, tpm20), requirements{tpm: true}, ""},
		{"tpm absent", caps(secureNo, ""), requirements{tpm: true}, "TPM 2.0"},
		{"tpm 1.2 only", caps(secureNo, tpm12), requirements{tpm: true}, "TPM 2.0"},
		{"tpm unsupported", caps(secureNo, `<tpm supported='no'/>`), requirements{tpm: true}, "TPM 2.0"},
		{"both ok", caps(secureYes, tpm20), requirements{secureBoot: true, tpm: true}, ""},
		{"not xml", "nope", requirements{}, "parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDomainCaps(tt.xml, tgt, tt.req)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCheckDomainCapsNoLoader(t *testing.T) {
	xml := `<domainCapabilities><os supported='yes'><loader supported='no'/></os></domainCapabilities>`
	if err := checkDomainCaps(xml, targetFor(ArchARM64), requirements{}); err == nil {
		t.Fatal("expected error when UEFI loaders are unsupported")
	}
}

func TestTargetFor(t *testing.T) {
	if got := targetFor(ArchAMD64); got.arch != "x86_64" || got.virtType != "kvm" || got.machine != "q35" {
		t.Errorf("amd64 target = %+v", got)
	}
	if got := targetFor(ArchARM64); got.arch != "aarch64" || got.virtType != "qemu" {
		t.Errorf("arm64 target = %+v", got)
	}
}
