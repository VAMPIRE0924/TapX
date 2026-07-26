package addressctrl

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"tapx/internal/model"
)

type Request struct {
	Key      string `json:"key"`
	Protocol string `json:"protocol,omitempty"`
}

type Lease struct {
	IPv4CIDR    string   `json:"ipv4Cidr,omitempty"`
	IPv6CIDR    string   `json:"ipv6Cidr,omitempty"`
	Gateway     string   `json:"gateway,omitempty"`
	DNS         []string `json:"dns,omitempty"`
	LeaseSecond int      `json:"leaseSecond"`
	ExpiresAt   int64    `json:"expiresAt"`
}

type Allocator struct {
	mu     sync.Mutex
	config model.TUNDHCPConfig
	byKey  map[string]Lease
	usedV4 map[netip.Addr]string
	usedV6 map[netip.Addr]string
	now    func() time.Time
}

func NewAllocator(config model.TUNDHCPConfig) (*Allocator, error) {
	if config.Mode != model.TUNDHCPModeServer {
		return nil, fmt.Errorf("addressctrl: TUN device is not in server mode")
	}
	allocator := &Allocator{
		config: config, byKey: make(map[string]Lease), usedV4: make(map[netip.Addr]string),
		usedV6: make(map[netip.Addr]string), now: time.Now,
	}
	if _, err := allocator.buildLease("__validate__", config.Protocol, false); err != nil {
		return nil, err
	}
	return allocator, nil
}

func (a *Allocator) Allocate(request Request) (Lease, error) {
	if a == nil {
		return Lease{}, errors.New("addressctrl: allocator is unavailable")
	}
	request.Key = strings.TrimSpace(request.Key)
	if request.Key == "" {
		return Lease{}, errors.New("addressctrl: lease key is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.releaseExpired(a.now().Unix(), request.Key)
	if lease, ok := a.byKey[request.Key]; ok && lease.ExpiresAt > a.now().Unix() {
		lease.ExpiresAt = a.now().Add(time.Duration(lease.LeaseSecond) * time.Second).Unix()
		a.byKey[request.Key] = lease
		return cloneLease(lease), nil
	}
	if lease, ok := a.byKey[request.Key]; ok {
		a.releaseLease(request.Key, lease)
	}
	lease, err := a.buildLease(request.Key, request.Protocol, true)
	if err != nil {
		return Lease{}, err
	}
	a.byKey[request.Key] = lease
	return cloneLease(lease), nil
}

func (a *Allocator) releaseExpired(now int64, exceptKey string) {
	for key, lease := range a.byKey {
		if key != exceptKey && lease.ExpiresAt <= now {
			a.releaseLease(key, lease)
		}
	}
}

func (a *Allocator) releaseLease(key string, lease Lease) {
	for _, cidr := range []string{lease.IPv4CIDR, lease.IPv6CIDR} {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		if prefix.Addr().Is4() {
			if a.usedV4[prefix.Addr()] == key {
				delete(a.usedV4, prefix.Addr())
			}
		} else if a.usedV6[prefix.Addr()] == key {
			delete(a.usedV6, prefix.Addr())
		}
	}
	delete(a.byKey, key)
}

func (a *Allocator) buildLease(key, requestedProtocol string, reserve bool) (Lease, error) {
	protocol := normalizeProtocol(requestedProtocol, a.config.Protocol)
	leaseSeconds := a.config.LeaseSeconds
	if leaseSeconds <= 0 {
		leaseSeconds = 86400
	}
	lease := Lease{
		Gateway: strings.TrimSpace(a.config.OfferedGateway), DNS: append([]string(nil), a.config.OfferedDNS...),
		LeaseSecond: leaseSeconds, ExpiresAt: a.now().Add(time.Duration(leaseSeconds) * time.Second).Unix(),
	}
	if protocol == "ipv4" || protocol == "dual" {
		prefix, err := netip.ParsePrefix(a.config.IPv4CIDR)
		if err != nil || !prefix.Addr().Is4() {
			return Lease{}, errors.New("addressctrl: valid IPv4 interface CIDR is required")
		}
		address, err := nextAddress(a.config.PoolStart, a.config.PoolEnd, a.usedV4)
		if err != nil {
			return Lease{}, fmt.Errorf("addressctrl: IPv4 pool: %w", err)
		}
		lease.IPv4CIDR = netip.PrefixFrom(address, prefix.Bits()).String()
		if reserve {
			a.usedV4[address] = key
		}
	}
	if protocol == "ipv6" || protocol == "dual" {
		prefix, err := netip.ParsePrefix(a.config.IPv6CIDR)
		if err != nil || !prefix.Addr().Is6() {
			return Lease{}, errors.New("addressctrl: valid IPv6 interface CIDR is required")
		}
		address, err := nextAddress(a.config.IPv6PoolStart, a.config.IPv6PoolEnd, a.usedV6)
		if err != nil {
			return Lease{}, fmt.Errorf("addressctrl: IPv6 pool: %w", err)
		}
		lease.IPv6CIDR = netip.PrefixFrom(address, prefix.Bits()).String()
		if reserve {
			a.usedV6[address] = key
		}
	}
	return lease, nil
}

func nextAddress(startValue, endValue string, used map[netip.Addr]string) (netip.Addr, error) {
	start, err := netip.ParseAddr(strings.TrimSpace(startValue))
	if err != nil {
		return netip.Addr{}, errors.New("pool start is invalid")
	}
	end, err := netip.ParseAddr(strings.TrimSpace(endValue))
	if err != nil || start.BitLen() != end.BitLen() || start.Compare(end) > 0 {
		return netip.Addr{}, errors.New("pool end is invalid")
	}
	for current := start; current.IsValid() && current.Compare(end) <= 0; current = current.Next() {
		if _, exists := used[current]; !exists {
			return current, nil
		}
	}
	return netip.Addr{}, errors.New("pool is exhausted")
}

func normalizeProtocol(requested, configured string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	configured = strings.ToLower(strings.TrimSpace(configured))
	if configured == "" {
		configured = "ipv4"
	}
	if requested == "" || requested == configured {
		return configured
	}
	if configured == "dual" && (requested == "ipv4" || requested == "ipv6") {
		return requested
	}
	return configured
}

func cloneLease(input Lease) Lease {
	input.DNS = append([]string(nil), input.DNS...)
	return input
}
