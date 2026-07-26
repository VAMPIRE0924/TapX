//go:build linux

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"tapx/internal/addressctrl"
	"tapx/internal/config"
	"tapx/internal/linkdiag"
	"tapx/internal/model"
	"tapx/internal/netapply"
)

var rawUDPAddressRequest = [8]byte{'T', 'X', 'A', 'D', 'D', 'R', '1', 0}

var rawUDPAddressResponseMagic = [8]byte{'T', 'X', 'A', 'D', 'D', 'R', '1', 1}

type deviceAddressControl struct {
	state       *deviceAddressControlState
	registryKey string
	closeOnce   sync.Once
	renewCancel context.CancelFunc
	renewDone   chan struct{}
}

type deviceAddressControlState struct {
	device   config.RuntimeDevice
	apply    netapply.Handle
	server   *addressctrl.Allocator
	mu       sync.Mutex
	applied  bool
	expires  int64
	renewing bool
}

var addressControlRegistry = struct {
	sync.Mutex
	items map[string]*addressControlRegistryEntry
}{items: make(map[string]*addressControlRegistryEntry)}

type addressControlRegistryEntry struct {
	config string
	state  *deviceAddressControlState
	refs   int
}

func newDeviceAddressControl(device config.RuntimeDevice, apply netapply.Handle) (*deviceAddressControl, error) {
	if device.Type != model.DeviceTUN {
		return nil, nil
	}
	configBytes, err := json.Marshal(device.TUNDHCP)
	if err != nil {
		return nil, fmt.Errorf("core: encode TUN address config: %w", err)
	}
	registryKey := device.ID
	if registryKey == "" {
		registryKey = device.IfName
	}
	addressControlRegistry.Lock()
	defer addressControlRegistry.Unlock()
	if current := addressControlRegistry.items[registryKey]; current != nil {
		if current.config != string(configBytes) {
			return nil, fmt.Errorf("core: device %s already has a different active address configuration", registryKey)
		}
		current.refs++
		return &deviceAddressControl{state: current.state, registryKey: registryKey}, nil
	}
	state := &deviceAddressControlState{device: device, apply: apply}
	switch device.TUNDHCP.Mode {
	case "", model.TUNDHCPModeOff, model.TUNDHCPModeManual:
		return nil, nil
	case model.TUNDHCPModeClient:
		if apply == nil {
			return nil, errors.New("core: TUN address client has no network apply handle")
		}
	case model.TUNDHCPModeServer:
		allocator, err := addressctrl.NewAllocator(device.TUNDHCP)
		if err != nil {
			return nil, err
		}
		state.server = allocator
	default:
		return nil, fmt.Errorf("core: unsupported TUN address mode %q", device.TUNDHCP.Mode)
	}
	addressControlRegistry.items[registryKey] = &addressControlRegistryEntry{config: string(configBytes), state: state, refs: 1}
	return &deviceAddressControl{state: state, registryKey: registryKey}, nil
}

func (c *deviceAddressControl) isClient() bool {
	return c != nil && c.state != nil && c.state.device.TUNDHCP.Mode == model.TUNDHCPModeClient
}

func (c *deviceAddressControl) isServer() bool {
	return c != nil && c.state != nil && c.state.server != nil
}

func (c *deviceAddressControl) streamOptions(credential string) linkdiag.StreamOptions {
	options := linkdiag.StreamOptions{Credential: credential}
	if c == nil || c.state == nil || c.state.server == nil {
		return options
	}
	options.AddressLease = func(request linkdiag.AddressLeaseRequest) (linkdiag.AddressLease, error) {
		lease, err := c.state.server.Allocate(addressctrl.Request{Key: request.Key, Protocol: request.Protocol})
		return linkdiag.AddressLease{
			IPv4CIDR: lease.IPv4CIDR, IPv6CIDR: lease.IPv6CIDR, Gateway: lease.Gateway,
			DNS: append([]string(nil), lease.DNS...), LeaseSecond: lease.LeaseSecond, ExpiresAt: lease.ExpiresAt,
		}, err
	}
	return options
}

func (c *deviceAddressControl) requestLease(ctx context.Context, conn net.Conn, credential, key string) error {
	if !c.isClient() {
		return nil
	}
	c.state.mu.Lock()
	if c.state.applied && (c.state.expires == 0 || c.state.expires > time.Now().Add(time.Minute).Unix()) {
		c.state.mu.Unlock()
		return nil
	}
	if c.state.renewing {
		c.state.mu.Unlock()
		return nil
	}
	c.state.renewing = true
	c.state.mu.Unlock()
	defer func() {
		c.state.mu.Lock()
		c.state.renewing = false
		c.state.mu.Unlock()
	}()
	lease, err := linkdiag.RequestAddressLease(ctx, conn, credential, linkdiag.AddressLeaseRequest{
		Key: key, Protocol: c.state.device.TUNDHCP.Protocol,
	})
	if err != nil {
		return err
	}
	return c.applyLease(lease)
}

func (c *deviceAddressControl) startRenewal(renew func(context.Context) error, report func(error)) {
	if !c.isClient() || renew == nil || c.renewCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.renewCancel = cancel
	c.renewDone = done
	go func() {
		defer close(done)
		for {
			timer := time.NewTimer(c.renewalDelay())
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
			if err := renew(ctx); err != nil && ctx.Err() == nil {
				if report != nil {
					report(err)
				}
				if !waitAddressRenewalRetry(ctx) {
					return
				}
			}
		}
	}()
}

func (c *deviceAddressControl) renewalDelay() time.Duration {
	const (
		minimum = time.Second
		idle    = time.Hour
	)
	if !c.isClient() {
		return idle
	}
	c.state.mu.Lock()
	applied, expires := c.state.applied, c.state.expires
	c.state.mu.Unlock()
	if !applied || expires == 0 {
		return idle
	}
	remaining := time.Until(time.Unix(expires, 0))
	margin := remaining / 5
	if margin < time.Minute {
		margin = time.Minute
	}
	delay := remaining - margin
	if delay < minimum {
		return minimum
	}
	return delay
}

func waitAddressRenewalRetry(ctx context.Context) bool {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *deviceAddressControl) applyLease(lease linkdiag.AddressLease) error {
	if !c.isClient() {
		return nil
	}
	if lease.IPv4CIDR == "" && lease.IPv6CIDR == "" {
		return errors.New("core: address service returned an empty lease")
	}
	if err := c.state.apply.ApplyAddressLease(netapply.AddressLease{
		IPv4CIDR: lease.IPv4CIDR, IPv6CIDR: lease.IPv6CIDR,
		Gateway: lease.Gateway, DNS: append([]string(nil), lease.DNS...),
		AllowDefaultRoute: c.state.device.AllowDefaultRoute,
	}); err != nil {
		return fmt.Errorf("core: apply TUN address lease: %w", err)
	}
	c.state.mu.Lock()
	c.state.applied = true
	c.state.expires = lease.ExpiresAt
	c.state.mu.Unlock()
	return nil
}

func (c *deviceAddressControl) rawUDPResponse(key string) ([]byte, error) {
	if !c.isServer() {
		return nil, nil
	}
	lease, err := c.state.server.Allocate(addressctrl.Request{Key: key, Protocol: c.state.device.TUNDHCP.Protocol})
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(linkdiag.AddressLease{
		IPv4CIDR: lease.IPv4CIDR, IPv6CIDR: lease.IPv6CIDR, Gateway: lease.Gateway,
		DNS: append([]string(nil), lease.DNS...), LeaseSecond: lease.LeaseSecond, ExpiresAt: lease.ExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("core: encode UDP address lease: %w", err)
	}
	response := make([]byte, 0, len(rawUDPAddressResponseMagic)+len(payload))
	response = append(response, rawUDPAddressResponseMagic[:]...)
	response = append(response, payload...)
	return response, nil
}

func (c *deviceAddressControl) serverLeasePrefixes(key string) ([]string, []string, error) {
	if !c.isServer() {
		return nil, nil, nil
	}
	lease, err := c.state.server.Allocate(addressctrl.Request{Key: key, Protocol: c.state.device.TUNDHCP.Protocol})
	if err != nil {
		return nil, nil, err
	}
	var ipv4, ipv6 []string
	if lease.IPv4CIDR != "" {
		ipv4 = append(ipv4, lease.IPv4CIDR)
	}
	if lease.IPv6CIDR != "" {
		ipv6 = append(ipv6, lease.IPv6CIDR)
	}
	return ipv4, ipv6, nil
}

func (c *deviceAddressControl) Close() {
	if c == nil || c.state == nil || c.registryKey == "" {
		return
	}
	c.closeOnce.Do(func() {
		if c.renewCancel != nil {
			c.renewCancel()
			c.renewCancel = nil
		}
		if c.renewDone != nil {
			<-c.renewDone
			c.renewDone = nil
		}
		addressControlRegistry.Lock()
		defer addressControlRegistry.Unlock()
		entry := addressControlRegistry.items[c.registryKey]
		if entry == nil || entry.state != c.state || entry.refs <= 0 {
			return
		}
		entry.refs--
		if entry.refs == 0 {
			delete(addressControlRegistry.items, c.registryKey)
		}
	})
}

func (c *deviceAddressControl) applyRawUDPResponse(payload []byte) error {
	if !c.isClient() {
		return nil
	}
	if len(payload) < len(rawUDPAddressResponseMagic) ||
		!equalBytes(payload[:len(rawUDPAddressResponseMagic)], rawUDPAddressResponseMagic[:]) {
		return errors.New("core: invalid UDP address lease response")
	}
	var lease linkdiag.AddressLease
	if err := json.Unmarshal(payload[len(rawUDPAddressResponseMagic):], &lease); err != nil {
		return fmt.Errorf("core: decode UDP address lease: %w", err)
	}
	return c.applyLease(lease)
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func serveAddressControl(ctx context.Context, conn net.Conn, credential string, control *deviceAddressControl) error {
	return linkdiag.ServeStreamWithOptions(ctx, conn, control.streamOptions(credential))
}

func addressLeaseKey(binding config.RuntimeBinding, deviceID, endpointID string) string {
	if binding.ClientID != "" {
		return "client:" + binding.ClientID
	}
	if binding.VKeyValue != "" {
		return "vkey:" + binding.VKeyValue
	}
	if deviceID != "" {
		return "device:" + deviceID
	}
	return "endpoint:" + endpointID
}
