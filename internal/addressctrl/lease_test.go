package addressctrl

import (
	"testing"
	"time"

	"tapx/internal/model"
)

func TestAllocatorReturnsStableDualStackLease(t *testing.T) {
	allocator, err := NewAllocator(model.TUNDHCPConfig{
		Mode: model.TUNDHCPModeServer, Protocol: "dual",
		IPv4CIDR: "10.20.0.1/24", PoolStart: "10.20.0.10", PoolEnd: "10.20.0.11",
		IPv6CIDR: "fd20::1/64", IPv6PoolStart: "fd20::10", IPv6PoolEnd: "fd20::11",
		OfferedGateway: "10.20.0.1", OfferedDNS: []string{"1.1.1.1"}, LeaseSeconds: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := allocator.Allocate(Request{Key: "client-a", Protocol: "dual"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := allocator.Allocate(Request{Key: "client-a", Protocol: "dual"})
	if err != nil {
		t.Fatal(err)
	}
	if first.IPv4CIDR != "10.20.0.10/24" || first.IPv6CIDR != "fd20::10/64" || again.IPv4CIDR != first.IPv4CIDR {
		t.Fatalf("unexpected leases: first=%+v again=%+v", first, again)
	}
	second, err := allocator.Allocate(Request{Key: "client-b", Protocol: "dual"})
	if err != nil {
		t.Fatal(err)
	}
	if second.IPv4CIDR != "10.20.0.11/24" || second.IPv6CIDR != "fd20::11/64" {
		t.Fatalf("unexpected second lease: %+v", second)
	}
}

func TestAllocatorReclaimsExpiredLease(t *testing.T) {
	allocator, err := NewAllocator(model.TUNDHCPConfig{
		Mode: model.TUNDHCPModeServer, Protocol: "ipv4",
		IPv4CIDR: "10.21.0.1/30", PoolStart: "10.21.0.2", PoolEnd: "10.21.0.2", LeaseSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	allocator.now = func() time.Time { return now }
	first, err := allocator.Allocate(Request{Key: "client-a"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(61 * time.Second)
	renewed, err := allocator.Allocate(Request{Key: "client-a"})
	if err != nil {
		t.Fatalf("renew expired lease: %v", err)
	}
	if renewed.IPv4CIDR != first.IPv4CIDR || renewed.ExpiresAt <= first.ExpiresAt {
		t.Fatalf("lease was not reclaimed and renewed: first=%+v renewed=%+v", first, renewed)
	}
}

func TestAllocatorRenewsActiveLease(t *testing.T) {
	allocator, err := NewAllocator(model.TUNDHCPConfig{
		Mode: model.TUNDHCPModeServer, Protocol: "ipv4",
		IPv4CIDR: "10.22.0.1/30", PoolStart: "10.22.0.2", PoolEnd: "10.22.0.2", LeaseSeconds: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	allocator.now = func() time.Time { return now }
	first, err := allocator.Allocate(Request{Key: "client-a"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(90 * time.Second)
	renewed, err := allocator.Allocate(Request{Key: "client-a"})
	if err != nil {
		t.Fatal(err)
	}
	if renewed.IPv4CIDR != first.IPv4CIDR || renewed.ExpiresAt != now.Add(120*time.Second).Unix() {
		t.Fatalf("active lease was not extended in place: first=%+v renewed=%+v", first, renewed)
	}
}

func TestAllocatorReclaimsExpiredLeaseForAnotherClient(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	allocator, err := NewAllocator(model.TUNDHCPConfig{
		Mode: model.TUNDHCPModeServer, Protocol: "ipv4",
		IPv4CIDR: "10.23.0.1/30", PoolStart: "10.23.0.2", PoolEnd: "10.23.0.2", LeaseSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	allocator.now = func() time.Time { return now }
	first, err := allocator.Allocate(Request{Key: "first", Protocol: "ipv4"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(61 * time.Second)
	second, err := allocator.Allocate(Request{Key: "second", Protocol: "ipv4"})
	if err != nil {
		t.Fatalf("expired lease was not reclaimed: %v", err)
	}
	if second.IPv4CIDR != first.IPv4CIDR {
		t.Fatalf("reclaimed address = %q, want %q", second.IPv4CIDR, first.IPv4CIDR)
	}
}
