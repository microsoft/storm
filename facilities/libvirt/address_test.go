package libvirt

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

func addrs(s ...string) []netip.Addr {
	var out []netip.Addr
	for _, a := range s {
		out = append(out, netip.MustParseAddr(a))
	}
	return out
}

// scripted returns a leaseQuery that replays the given answers in order and
// repeats the last one.
func scripted(answers ...[]netip.Addr) (leaseQuery, *int) {
	calls := 0
	return func(context.Context) ([]netip.Addr, error) {
		i := calls
		if i >= len(answers) {
			i = len(answers) - 1
		}
		calls++
		return answers[i], nil
	}, &calls
}

var fastStabilize = stabilizeConfig{retries: 5, interval: 0}

func TestIPv4Leases(t *testing.T) {
	ifaces := []golibvirt.DomainInterface{
		{
			Name:   "vnet0",
			Hwaddr: golibvirt.OptString{"52:54:00:AA:BB:CC"},
			Addrs: []golibvirt.DomainIPAddr{
				{Type: int32(golibvirt.IPAddrTypeIpv4), Addr: "192.168.77.10", Prefix: 24},
				{Type: int32(golibvirt.IPAddrTypeIpv6), Addr: "fe80::1", Prefix: 64},
				{Type: int32(golibvirt.IPAddrTypeIpv4), Addr: "not-an-ip"},
				{Type: int32(golibvirt.IPAddrTypeIpv4), Addr: "192.168.77.11", Prefix: 24},
			},
		},
		{
			Name:   "other",
			Hwaddr: golibvirt.OptString{"52:54:00:00:00:99"},
			Addrs:  []golibvirt.DomainIPAddr{{Type: int32(golibvirt.IPAddrTypeIpv4), Addr: "10.0.0.5"}},
		},
		{Name: "nohw", Addrs: []golibvirt.DomainIPAddr{{Type: int32(golibvirt.IPAddrTypeIpv4), Addr: "192.168.77.12"}}},
	}
	got := ipv4Leases(ifaces, "52:54:00:aa:bb:cc")
	want := addrs("192.168.77.10", "192.168.77.11", "192.168.77.12")
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	if got := ipv4Leases(nil, "x"); len(got) != 0 {
		t.Errorf("empty input gave %v", got)
	}
}

func TestResolveAddressNone(t *testing.T) {
	q, calls := scripted(nil)
	_, err := resolveAddress(context.Background(), q, fastStabilize)
	if !errors.Is(err, ErrNoAddress) {
		t.Fatalf("err = %v, want ErrNoAddress", err)
	}
	if *calls != 1 {
		t.Errorf("queried %d times, want 1 (no internal polling)", *calls)
	}
}

func TestResolveAddressQueryError(t *testing.T) {
	boom := errors.New("boom")
	q := func(context.Context) ([]netip.Addr, error) { return nil, boom }
	_, err := resolveAddress(context.Background(), q, fastStabilize)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping boom", err)
	}
}

func TestResolveAddressSingle(t *testing.T) {
	q, calls := scripted(addrs("192.168.77.10"))
	got, err := resolveAddress(context.Background(), q, fastStabilize)
	if err != nil || got != netip.MustParseAddr("192.168.77.10") {
		t.Fatalf("got %v, %v", got, err)
	}
	if *calls != 1 {
		t.Errorf("queried %d times, want 1", *calls)
	}
}

func TestResolveAddressStabilises(t *testing.T) {
	q, calls := scripted(
		addrs("192.168.77.10", "192.168.77.11"),
		addrs("192.168.77.10", "192.168.77.11"),
		nil, // a transient empty answer is skipped
		addrs("192.168.77.11"),
	)
	got, err := resolveAddress(context.Background(), q, fastStabilize)
	if err != nil || got != netip.MustParseAddr("192.168.77.11") {
		t.Fatalf("got %v, %v", got, err)
	}
	if *calls != 4 {
		t.Errorf("queried %d times, want 4", *calls)
	}
}

func TestResolveAddressNeverStabilises(t *testing.T) {
	q, calls := scripted(addrs("192.168.77.20", "192.168.77.21"))
	got, err := resolveAddress(context.Background(), q, fastStabilize)
	if err != nil || got != netip.MustParseAddr("192.168.77.20") {
		t.Fatalf("got %v, %v", got, err)
	}
	if want := 1 + fastStabilize.retries; *calls != want {
		t.Errorf("queried %d times, want %d (bounded)", *calls, want)
	}
}

func TestResolveAddressContextCancelledWhileStabilising(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := func(context.Context) ([]netip.Addr, error) {
		cancel()
		return addrs("192.168.77.20", "192.168.77.21"), nil
	}
	start := time.Now()
	_, err := resolveAddress(ctx, q, stabilizeConfig{retries: 5, interval: time.Hour})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("did not return promptly on cancellation")
	}
}

func TestVMAddressUsesInjectedQuery(t *testing.T) {
	q, _ := scripted(nil)
	v := &VM{plan: domainPlan{name: "ns_vm0"}, leases: q, stabilize: fastStabilize}
	_, err := v.Address(context.Background())
	if !errors.Is(err, ErrNoAddress) {
		t.Fatalf("err = %v, want ErrNoAddress", err)
	}

	v.leases, _ = scripted(addrs("192.168.77.5"))
	got, err := v.Address(context.Background())
	if err != nil || got != netip.MustParseAddr("192.168.77.5") {
		t.Fatalf("got %v, %v", got, err)
	}
}
