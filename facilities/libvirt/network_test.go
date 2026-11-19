package libvirt

import (
	"net/netip"
	"strings"
	"testing"
)

func mustPrefix(s string) netip.Prefix { return netip.MustParsePrefix(s) }

func TestChooseSubnetDeterministic(t *testing.T) {
	a, err := chooseSubnet("alpha", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := chooseSubnet("alpha", nil)
	if a != b {
		t.Errorf("not deterministic: %s vs %s", a, b)
	}
	if a.Bits() != 24 || !mustPrefix("192.168.0.0/16").Contains(a.Addr()) {
		t.Errorf("subnet %s outside 192.168.0.0/16 /24", a)
	}
	c, _ := chooseSubnet("beta", nil)
	if a == c {
		t.Errorf("namespaces alpha and beta both got %s", a)
	}
}

func TestChooseSubnetAvoidsUsed(t *testing.T) {
	first, _ := chooseSubnet("alpha", nil)

	// Block the preferred subnet with an exact match.
	got, err := chooseSubnet("alpha", []netip.Prefix{first})
	if err != nil {
		t.Fatal(err)
	}
	if got == first || got.Overlaps(first) {
		t.Errorf("got %s, which overlaps used %s", got, first)
	}

	// A /22 covering the preferred subnet must also be avoided.
	wide := netip.PrefixFrom(first.Addr(), 22).Masked()
	got, err = chooseSubnet("alpha", []netip.Prefix{wide})
	if err != nil {
		t.Fatal(err)
	}
	if got.Overlaps(wide) {
		t.Errorf("got %s, which overlaps used %s", got, wide)
	}
}

func TestChooseSubnetNeverUsesReserved(t *testing.T) {
	reserved := []netip.Prefix{mustPrefix("192.168.0.0/24"), mustPrefix("192.168.1.0/24"), mustPrefix("192.168.122.0/24")}
	for i := range 2000 {
		ns := ("ns" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26)))
		got, err := chooseSubnet(ns, nil)
		if err != nil {
			t.Fatal(err)
		}
		if overlapsAny(got, reserved) {
			t.Fatalf("namespace %q got reserved subnet %s", ns, got)
		}
	}
}

func TestChooseSubnetExhausted(t *testing.T) {
	if _, err := chooseSubnet("alpha", []netip.Prefix{mustPrefix("192.168.0.0/16")}); err == nil {
		t.Error("expected error when 192.168.0.0/16 is fully used")
	}
}

func TestTwoNamespacesGetDistinctSubnetsViaUsed(t *testing.T) {
	// Simulates the second client seeing the first client's network.
	a, _ := chooseSubnet("same", nil)
	b, err := chooseSubnet("same", []netip.Prefix{a})
	if err != nil {
		t.Fatal(err)
	}
	if a.Overlaps(b) {
		t.Errorf("subnets overlap: %s %s", a, b)
	}
}

func TestNetworkXMLRoundTrip(t *testing.T) {
	xml, err := networkXML("ns_net", mustPrefix("192.168.77.0/24"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := networkPrefixes(xml)
	if err != nil {
		t.Fatal(err)
	}
	// The gateway address 192.168.77.1 is inside the /24.
	if len(ps) != 1 || ps[0] != mustPrefix("192.168.77.0/24") {
		t.Errorf("prefixes = %v", ps)
	}
	for _, want := range []string{`mode="nat"`, `192.168.77.2`, `192.168.77.254`, `255.255.255.0`} {
		if !strings.Contains(xml, want) {
			t.Errorf("network XML missing %q:\n%s", want, xml)
		}
	}
}

func TestNetworkPrefixesPrefixAttr(t *testing.T) {
	xml := `<network><name>x</name><ip address="10.1.2.1" prefix="16"/><ip family="ipv6" address="fd00::1" prefix="64"/></network>`
	ps, err := networkPrefixes(xml)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0] != mustPrefix("10.1.0.0/16") {
		t.Errorf("prefixes = %v", ps)
	}
}
