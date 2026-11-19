package libvirt

import (
	"slices"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	good := []string{"a", "stormtest-1a2b", "ci", "a-b-c", "0"}
	bad := []string{"", "Upper", "under_score", "-lead", "trail-", "dou--ble", "sp ace", "dot.ted", strings.Repeat("a", maxNameLen+1)}
	for _, s := range good {
		if err := validateName("namespace", s); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range bad {
		if err := validateName("namespace", s); err == nil {
			t.Errorf("validateName(%q) = nil, want error", s)
		}
	}
}

func TestValidateRunID(t *testing.T) {
	for _, s := range []string{"a", "ab12cd34", "0123456789abcdef"} {
		if err := validateRunID(s); err != nil {
			t.Errorf("validateRunID(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range []string{"", "AB", "a_b", "a-b", "0123456789abcdef0"} {
		if err := validateRunID(s); err == nil {
			t.Errorf("validateRunID(%q) = nil, want error", s)
		}
	}
}

func TestNewRunIDIsValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		id, err := newRunID()
		if err != nil {
			t.Fatal(err)
		}
		if err := validateRunID(id); err != nil {
			t.Fatalf("generated invalid run ID %q: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate run ID %q", id)
		}
		seen[id] = true
	}
}

func TestScopeNames(t *testing.T) {
	sc := scope{ns: "ci", run: "ab12"}
	cases := map[string]string{
		sc.networkName():                     "ci_ab12_net",
		sc.poolName():                        "ci_ab12_pool",
		sc.poolPath():                        "/var/lib/libvirt/ci_ab12_pool",
		sc.domainName("vm0"):                 "ci_ab12_vm0",
		sc.volumeName("vm0", 1, formatQcow2): "ci_ab12_vm0-disk1.qcow2",
		sc.volumeName("vm0", 0, formatRaw):   "ci_ab12_vm0-disk0.img",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestScopeOwns(t *testing.T) {
	a := scope{ns: "ci", run: "aa11"}
	b := scope{ns: "ci", run: "bb22"}
	c := scope{ns: "ci-2", run: "aa11"}
	d := scope{ns: "ci", run: "aa111"} // run ID that extends another one
	all := []scope{a, b, c, d}
	for _, owner := range all {
		objs := []string{owner.networkName(), owner.poolName(), owner.domainName("x"), owner.volumeName("x", 0, formatQcow2)}
		for _, n := range objs {
			for _, other := range all {
				if got, want := other.owns(n), other == owner; got != want {
					t.Errorf("%s.owns(%q) = %t, want %t", other, n, got, want)
				}
			}
		}
	}
	for _, foreign := range []string{"default", "somevm", "ci", "ci_aa11", "cinet", "ci-aa11-net", "other_ci_aa11_net", "ci_net"} {
		if a.owns(foreign) {
			t.Errorf("%s must not own %q", a, foreign)
		}
	}
}

func TestRunsIn(t *testing.T) {
	names := []string{
		"ci_bb22_net", "ci_aa11_vm0", "ci_aa11_pool", "ci_aa11_vm0-disk0.qcow2",
		"ci-2_cc33_net",     // other namespace
		"ci_net",            // no run segment
		"ci_BAD_x",          // invalid run ID
		"default", "somevm", // unrelated
		"xci_dd44_net", // namespace only as a suffix
	}
	got := runsIn("ci", names)
	want := []string{"bb22", "aa11"}
	if !slices.Equal(got, want) {
		t.Errorf("runsIn = %v, want %v", got, want)
	}
	if got := runsIn("ci-2", names); !slices.Equal(got, []string{"cc33"}) {
		t.Errorf("runsIn(ci-2) = %v", got)
	}
}
