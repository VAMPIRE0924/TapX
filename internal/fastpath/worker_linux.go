//go:build linux && cgo

package fastpath

/*
#cgo CFLAGS: -I${SRCDIR}/../../core/fastpath/include -std=c11 -O3 -Wall -Wextra -Werror -fPIC -pthread
#cgo LDFLAGS: -pthread
#include <arpa/inet.h>
#include <stdlib.h>
#include <string.h>
#include "tapx_fastpath.h"

static void tapx_sockaddr4(struct sockaddr_storage *storage,
    unsigned char a0, unsigned char a1, unsigned char a2, unsigned char a3,
    unsigned short port) {
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    addr.sin_addr.s_addr = htonl(((uint32_t)a0 << 24) | ((uint32_t)a1 << 16) | ((uint32_t)a2 << 8) | (uint32_t)a3);
    memset(storage, 0, sizeof(*storage));
    memcpy(storage, &addr, sizeof(addr));
}

static void tapx_sockaddr6(struct sockaddr_storage *storage,
    const unsigned char *addr_bytes, unsigned short port) {
    struct sockaddr_in6 addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin6_family = AF_INET6;
    addr.sin6_port = htons(port);
    memcpy(&addr.sin6_addr, addr_bytes, 16);
    memset(storage, 0, sizeof(*storage));
    memcpy(storage, &addr, sizeof(addr));
}
*/
import "C"

import (
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"syscall"
	"unsafe"

	"tapx/internal/model"
)

type FrameKind uint32

const (
	FrameTUN FrameKind = C.TAPX_FRAME_TUN
	FrameTAP FrameKind = C.TAPX_FRAME_TAP
)

type UDPPeerMode uint32

const (
	UDPPeerAny   UDPPeerMode = C.TAPX_UDP_PEER_ANY
	UDPPeerFixed UDPPeerMode = C.TAPX_UDP_PEER_FIXED
	UDPPeerLearn UDPPeerMode = C.TAPX_UDP_PEER_LEARN
)

type TCPLengthMode uint32

const (
	TCPLength16 TCPLengthMode = C.TAPX_TCP_LENGTH_UINT16
	TCPLength32 TCPLengthMode = C.TAPX_TCP_LENGTH_UINT32
)

type CountersSnapshot struct {
	RXPackets  uint64
	TXPackets  uint64
	RXBytes    uint64
	TXBytes    uint64
	DropsGuard uint64
	DropsIO    uint64
}

type AddressGuard struct {
	IPv4Prefixes []netip.Prefix
	IPv6Prefixes []netip.Prefix
	MACs         [][6]byte
}

type Counters struct {
	ptr *C.struct_tapx_fastpath_counters
}

func NewCounters() *Counters {
	ptr := (*C.struct_tapx_fastpath_counters)(C.calloc(1, C.size_t(C.sizeof_struct_tapx_fastpath_counters)))
	counters := &Counters{ptr: ptr}
	counters.Reset()
	runtime.SetFinalizer(counters, (*Counters).Close)
	return counters
}

func (c *Counters) Reset() {
	if c == nil || c.ptr == nil {
		return
	}
	C.tapx_fastpath_counters_reset(c.ptr)
}

func (c *Counters) Close() {
	if c == nil || c.ptr == nil {
		return
	}
	C.free(unsafe.Pointer(c.ptr))
	c.ptr = nil
}

func (c *Counters) Snapshot() CountersSnapshot {
	if c == nil || c.ptr == nil {
		return CountersSnapshot{}
	}
	var snapshot C.struct_tapx_fastpath_counters
	C.tapx_fastpath_counters_snapshot(c.ptr, &snapshot)
	return CountersSnapshot{
		RXPackets:  uint64(snapshot.rx_packets),
		TXPackets:  uint64(snapshot.tx_packets),
		RXBytes:    uint64(snapshot.rx_bytes),
		TXBytes:    uint64(snapshot.tx_bytes),
		DropsGuard: uint64(snapshot.drops_guard),
		DropsIO:    uint64(snapshot.drops_io),
	}
}

type UDPConfig struct {
	TUNFD        int
	UDPFD        int
	FrameKind    FrameKind
	MaxFrameSize uint32
	// MaxDatagramPayload enables the TapX segment format when non-zero. It is
	// the peer-confirmed outer UDP payload ceiling, including vKey and segment
	// headers but excluding the outer IP/UDP headers.
	MaxDatagramPayload     uint32
	PeerMode               UDPPeerMode
	InitialHandshake       bool
	KeepAliveInterval      uint32
	IdleTimeout            uint32
	AddressGuardRemote     bool
	Peer                   netip.AddrPort
	DeviceToNetworkRateBPS uint64
	NetworkToDeviceRateBPS uint64
	VKey                   []byte
	AddressResponse        []byte
	AddressGuard           AddressGuard
	Counters               *Counters
}

type TCPConfig struct {
	TUNFD                  int
	TCPFD                  int
	FrameKind              FrameKind
	MaxFrameSize           uint32
	LengthMode             TCPLengthMode
	IdleTimeout            uint32
	AddressGuardRemote     bool
	DeviceToNetworkRateBPS uint64
	NetworkToDeviceRateBPS uint64
	VKey                   []byte
	AddressGuard           AddressGuard
	Counters               *Counters
}

type DeviceSwitchPortConfig struct {
	FD     int
	Routes AddressGuard
}

type DeviceSwitchConfig struct {
	DeviceFD     int
	FrameKind    FrameKind
	MaxFrameSize uint32
	Ports        []DeviceSwitchPortConfig
	Counters     *Counters
}

type Worker struct {
	ptr      *C.struct_tapx_worker
	counters *Counters
}

type DeviceSwitch struct {
	ptr      *C.struct_tapx_device_switch
	counters *Counters
}

func ABI() uint32 {
	return uint32(C.tapx_fastpath_abi_version())
}

func FrameKindFromDevice(deviceType model.DeviceType) (FrameKind, error) {
	switch deviceType {
	case model.DeviceTUN:
		return FrameTUN, nil
	case model.DeviceTAP:
		return FrameTAP, nil
	default:
		return 0, fmt.Errorf("fastpath: unsupported device type %q", deviceType)
	}
}

func TCPLengthModeFromModel(mode model.TCPLengthMode) (TCPLengthMode, error) {
	switch mode {
	case "", model.TCPLength16:
		return TCPLength16, nil
	case model.TCPLength32:
		return TCPLength32, nil
	default:
		return 0, fmt.Errorf("fastpath: unsupported tcp length mode %q", mode)
	}
}

func StartUDPPipe(cfg UDPConfig) (*Worker, error) {
	if cfg.Counters == nil {
		cfg.Counters = NewCounters()
	}
	if cfg.Counters.ptr == nil {
		return nil, errors.New("fastpath: counters are closed")
	}

	var c C.struct_tapx_udp_pipe_config
	c.tun_fd = C.int(cfg.TUNFD)
	c.udp_fd = C.int(cfg.UDPFD)
	c.frame_kind = C.uint32_t(cfg.FrameKind)
	c.max_frame_size = C.uint32_t(cfg.MaxFrameSize)
	c.max_datagram_payload = C.uint32_t(cfg.MaxDatagramPayload)
	c.peer_mode = C.uint32_t(cfg.PeerMode)
	if cfg.InitialHandshake {
		c.initial_handshake = 1
	}
	c.keepalive_interval_ms = C.uint32_t(cfg.KeepAliveInterval)
	c.idle_timeout_ms = C.uint32_t(cfg.IdleTimeout)
	if cfg.AddressGuardRemote {
		c.address_guard_remote = 1
	}
	c.device_to_network_rate_bps = C.uint64_t(cfg.DeviceToNetworkRateBPS)
	c.network_to_device_rate_bps = C.uint64_t(cfg.NetworkToDeviceRateBPS)
	c.counters = cfg.Counters.ptr
	if cfg.Peer.IsValid() {
		if err := fillSockaddr(&c, cfg.Peer); err != nil {
			return nil, err
		}
	}
	freeGuard, err := fillAddressGuard(&c.guard, cfg.AddressGuard)
	if err != nil {
		return nil, err
	}
	defer freeGuard()
	freeVKey, err := fillVKeyGuard(&c.vkey, cfg.VKey)
	if err != nil {
		return nil, err
	}
	defer freeVKey()
	var freeAddressResponse func()
	if len(cfg.AddressResponse) > 0 {
		ptr := C.CBytes(cfg.AddressResponse)
		if ptr == nil {
			return nil, errors.New("fastpath: allocate UDP address response")
		}
		c.address_response = (*C.uint8_t)(ptr)
		c.address_response_len = C.size_t(len(cfg.AddressResponse))
		freeAddressResponse = func() { C.free(ptr) }
	} else {
		freeAddressResponse = func() {}
	}
	defer freeAddressResponse()

	var ptr *C.struct_tapx_worker
	rc := C.tapx_udp_pipe_start(&c, &ptr)
	if rc != 0 {
		return nil, errnoError("start udp pipe", rc)
	}
	return &Worker{ptr: ptr, counters: cfg.Counters}, nil
}

func StartTCPPipe(cfg TCPConfig) (*Worker, error) {
	if cfg.Counters == nil {
		cfg.Counters = NewCounters()
	}
	if cfg.Counters.ptr == nil {
		return nil, errors.New("fastpath: counters are closed")
	}

	var c C.struct_tapx_tcp_pipe_config
	c.tun_fd = C.int(cfg.TUNFD)
	c.tcp_fd = C.int(cfg.TCPFD)
	c.frame_kind = C.uint32_t(cfg.FrameKind)
	c.max_frame_size = C.uint32_t(cfg.MaxFrameSize)
	c.length_mode = C.uint32_t(cfg.LengthMode)
	c.idle_timeout_ms = C.uint32_t(cfg.IdleTimeout)
	if cfg.AddressGuardRemote {
		c.address_guard_remote = 1
	}
	c.device_to_network_rate_bps = C.uint64_t(cfg.DeviceToNetworkRateBPS)
	c.network_to_device_rate_bps = C.uint64_t(cfg.NetworkToDeviceRateBPS)
	c.counters = cfg.Counters.ptr
	freeGuard, err := fillAddressGuard(&c.guard, cfg.AddressGuard)
	if err != nil {
		return nil, err
	}
	defer freeGuard()
	freeVKey, err := fillVKeyGuard(&c.vkey, cfg.VKey)
	if err != nil {
		return nil, err
	}
	defer freeVKey()

	var ptr *C.struct_tapx_worker
	rc := C.tapx_tcp_pipe_start(&c, &ptr)
	if rc != 0 {
		return nil, errnoError("start tcp pipe", rc)
	}
	return &Worker{ptr: ptr, counters: cfg.Counters}, nil
}

func StartDeviceSwitch(cfg DeviceSwitchConfig) (*DeviceSwitch, error) {
	if len(cfg.Ports) == 0 {
		return nil, errors.New("fastpath: device switch requires at least one port")
	}
	if cfg.Counters == nil {
		cfg.Counters = NewCounters()
	}
	if cfg.Counters.ptr == nil {
		return nil, errors.New("fastpath: counters are closed")
	}
	portsPtr := C.calloc(C.size_t(len(cfg.Ports)), C.size_t(C.sizeof_struct_tapx_device_switch_port))
	if portsPtr == nil {
		return nil, errors.New("fastpath: allocate device switch ports")
	}
	defer C.free(portsPtr)
	ports := unsafe.Slice((*C.struct_tapx_device_switch_port)(portsPtr), len(cfg.Ports))
	cleanups := make([]func(), 0, len(cfg.Ports))
	defer func() {
		for index := len(cleanups) - 1; index >= 0; index-- {
			cleanups[index]()
		}
	}()
	for index, port := range cfg.Ports {
		ports[index].fd = C.int(port.FD)
		var guard C.struct_tapx_address_guard
		cleanup, err := fillAddressGuard(&guard, AddressGuard{
			IPv4Prefixes: port.Routes.IPv4Prefixes,
			IPv6Prefixes: port.Routes.IPv6Prefixes,
		})
		if err != nil {
			return nil, err
		}
		cleanups = append(cleanups, cleanup)
		ports[index].ipv4_prefixes = guard.ipv4_prefixes
		ports[index].ipv4_prefix_count = guard.ipv4_prefix_count
		ports[index].ipv6_prefixes = guard.ipv6_prefixes
		ports[index].ipv6_prefix_count = guard.ipv6_prefix_count
	}
	config := C.struct_tapx_device_switch_config{
		device_fd: C.int(cfg.DeviceFD), frame_kind: C.uint32_t(cfg.FrameKind),
		max_frame_size: C.uint32_t(cfg.MaxFrameSize),
		ports:          (*C.struct_tapx_device_switch_port)(portsPtr), port_count: C.size_t(len(cfg.Ports)),
		counters: cfg.Counters.ptr,
	}
	var ptr *C.struct_tapx_device_switch
	rc := C.tapx_device_switch_start(&config, &ptr)
	if rc != 0 {
		return nil, errnoError("start device switch", rc)
	}
	return &DeviceSwitch{ptr: ptr, counters: cfg.Counters}, nil
}

func (w *Worker) Stop() error {
	if w == nil || w.ptr == nil {
		return nil
	}
	rc := C.tapx_worker_stop(w.ptr)
	w.ptr = nil
	if rc != 0 {
		return errnoError("stop worker", rc)
	}
	return nil
}

func (w *Worker) Counters() CountersSnapshot {
	if w == nil || w.counters == nil {
		return CountersSnapshot{}
	}
	return w.counters.Snapshot()
}

func (s *DeviceSwitch) Stop() error {
	if s == nil || s.ptr == nil {
		return nil
	}
	rc := C.tapx_device_switch_stop(s.ptr)
	s.ptr = nil
	if rc != 0 {
		return errnoError("stop device switch", rc)
	}
	return nil
}

func (s *DeviceSwitch) Counters() CountersSnapshot {
	if s == nil || s.counters == nil {
		return CountersSnapshot{}
	}
	return s.counters.Snapshot()
}

func fillSockaddr(c *C.struct_tapx_udp_pipe_config, peer netip.AddrPort) error {
	addr := peer.Addr().Unmap()
	if addr.Is4() {
		raw := addr.As4()
		C.tapx_sockaddr4(&c.peer_addr,
			C.uchar(raw[0]), C.uchar(raw[1]), C.uchar(raw[2]), C.uchar(raw[3]),
			C.ushort(peer.Port()))
		c.peer_addr_len = C.socklen_t(C.sizeof_struct_sockaddr_in)
		return nil
	}
	if addr.Is6() {
		raw := addr.As16()
		C.tapx_sockaddr6(&c.peer_addr, (*C.uchar)(unsafe.Pointer(&raw[0])), C.ushort(peer.Port()))
		c.peer_addr_len = C.socklen_t(C.sizeof_struct_sockaddr_in6)
		return nil
	}
	return errors.New("fastpath: peer address must be IPv4 or IPv6")
}

func fillVKeyGuard(c *C.struct_tapx_vkey_guard, value []byte) (func(), error) {
	if len(value) == 0 {
		return func() {}, nil
	}
	if len(value) > 1024 {
		return nil, fmt.Errorf("fastpath: vKey length %d exceeds 1024 bytes", len(value))
	}
	ptr := C.CBytes(value)
	if ptr == nil {
		return nil, errors.New("fastpath: allocate vKey")
	}
	c.value = (*C.uint8_t)(ptr)
	c.value_len = C.size_t(len(value))
	return func() { C.free(ptr) }, nil
}

func fillAddressGuard(c *C.struct_tapx_address_guard, guard AddressGuard) (func(), error) {
	if len(guard.IPv4Prefixes) == 0 && len(guard.IPv6Prefixes) == 0 && len(guard.MACs) == 0 {
		return func() {}, nil
	}
	var ipv4Ptr unsafe.Pointer
	var ipv6Ptr unsafe.Pointer
	var macPtr unsafe.Pointer
	free := func() {
		if ipv4Ptr != nil {
			C.free(ipv4Ptr)
		}
		if ipv6Ptr != nil {
			C.free(ipv6Ptr)
		}
		if macPtr != nil {
			C.free(macPtr)
		}
	}

	if len(guard.IPv4Prefixes) > 0 {
		ipv4Ptr = C.calloc(C.size_t(len(guard.IPv4Prefixes)), C.size_t(C.sizeof_struct_tapx_ipv4_prefix))
		if ipv4Ptr == nil {
			return nil, errors.New("fastpath: allocate IPv4 address guard")
		}
		prefixes := unsafe.Slice((*C.struct_tapx_ipv4_prefix)(ipv4Ptr), len(guard.IPv4Prefixes))
		for i, prefix := range guard.IPv4Prefixes {
			network, mask, err := ipv4Prefix(prefix)
			if err != nil {
				free()
				return nil, err
			}
			prefixes[i].network = C.uint32_t(network)
			prefixes[i].mask = C.uint32_t(mask)
		}
		c.ipv4_prefixes = (*C.struct_tapx_ipv4_prefix)(ipv4Ptr)
		c.ipv4_prefix_count = C.size_t(len(guard.IPv4Prefixes))
	}

	if len(guard.IPv6Prefixes) > 0 {
		ipv6Ptr = C.calloc(C.size_t(len(guard.IPv6Prefixes)), C.size_t(C.sizeof_struct_tapx_ipv6_prefix))
		if ipv6Ptr == nil {
			free()
			return nil, errors.New("fastpath: allocate IPv6 address guard")
		}
		prefixes := unsafe.Slice((*C.struct_tapx_ipv6_prefix)(ipv6Ptr), len(guard.IPv6Prefixes))
		for i, prefix := range guard.IPv6Prefixes {
			network, mask, err := ipv6Prefix(prefix)
			if err != nil {
				free()
				return nil, err
			}
			for j := range network {
				prefixes[i].network[j] = C.uint8_t(network[j])
				prefixes[i].mask[j] = C.uint8_t(mask[j])
			}
		}
		c.ipv6_prefixes = (*C.struct_tapx_ipv6_prefix)(ipv6Ptr)
		c.ipv6_prefix_count = C.size_t(len(guard.IPv6Prefixes))
	}

	if len(guard.MACs) > 0 {
		macPtr = C.calloc(C.size_t(len(guard.MACs)), C.size_t(C.sizeof_struct_tapx_mac_addr))
		if macPtr == nil {
			free()
			return nil, errors.New("fastpath: allocate MAC address guard")
		}
		macs := unsafe.Slice((*C.struct_tapx_mac_addr)(macPtr), len(guard.MACs))
		for i, mac := range guard.MACs {
			for j := range mac {
				macs[i].bytes[j] = C.uint8_t(mac[j])
			}
		}
		c.macs = (*C.struct_tapx_mac_addr)(macPtr)
		c.mac_count = C.size_t(len(guard.MACs))
	}
	return free, nil
}

func ipv4Prefix(prefix netip.Prefix) (uint32, uint32, error) {
	if !prefix.IsValid() || !prefix.Addr().Is4() {
		return 0, 0, fmt.Errorf("fastpath: address guard prefix %q is not IPv4", prefix.String())
	}
	bits := prefix.Bits()
	if bits < 0 || bits > 32 {
		return 0, 0, fmt.Errorf("fastpath: address guard prefix %q has invalid bits", prefix.String())
	}
	mask := uint32(0)
	if bits > 0 {
		mask = ^uint32(0) << uint(32-bits)
	}
	addr := prefix.Masked().Addr().As4()
	network := (uint32(addr[0]) << 24) |
		(uint32(addr[1]) << 16) |
		(uint32(addr[2]) << 8) |
		uint32(addr[3])
	return network & mask, mask, nil
}

func ipv6Prefix(prefix netip.Prefix) ([16]byte, [16]byte, error) {
	if !prefix.IsValid() || !prefix.Addr().Is6() {
		return [16]byte{}, [16]byte{}, fmt.Errorf("fastpath: address guard prefix %q is not IPv6", prefix.String())
	}
	bits := prefix.Bits()
	if bits < 0 || bits > 128 {
		return [16]byte{}, [16]byte{}, fmt.Errorf("fastpath: address guard prefix %q has invalid bits", prefix.String())
	}
	addr := prefix.Masked().Addr().As16()
	var mask [16]byte
	for i := 0; i < bits; i++ {
		mask[i/8] |= 1 << uint(7-(i%8))
	}
	var network [16]byte
	for i := range network {
		network[i] = addr[i] & mask[i]
	}
	return network, mask, nil
}

func errnoError(op string, rc C.int) error {
	code := int(rc)
	if code < 0 {
		code = -code
	}
	if code == 0 {
		return nil
	}
	return fmt.Errorf("fastpath: %s: %w", op, syscall.Errno(code))
}
