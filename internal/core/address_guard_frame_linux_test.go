//go:build linux

package core

import (
	"net/netip"
	"testing"

	"tapx/internal/fastpath"
)

func TestTapFrameAllowedVLANAndQinQIPv4(t *testing.T) {
	guard := fastpath.AddressGuard{IPv4Prefixes: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/24")}}
	for _, tags := range [][]uint16{{0x8100}, {0x88a8, 0x8100}} {
		allowed := ethernetIPv4Frame(tags, [4]byte{10, 20, 0, 2}, [4]byte{10, 30, 0, 2})
		if !tapFrameAllowed(allowed, guard, true) {
			t.Fatalf("source address was rejected for tags %x", tags)
		}
		disallowed := ethernetIPv4Frame(tags, [4]byte{10, 21, 0, 2}, [4]byte{10, 30, 0, 2})
		if tapFrameAllowed(disallowed, guard, true) {
			t.Fatalf("source address bypassed guard for tags %x", tags)
		}
	}
}

func TestTapFrameAllowedPPPoESessionIPv4(t *testing.T) {
	guard := fastpath.AddressGuard{IPv4Prefixes: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/24")}}
	allowed := pppoeIPv4Frame([4]byte{198, 51, 100, 1}, [4]byte{10, 20, 0, 2})
	if !tapFrameAllowed(allowed, guard, false) {
		t.Fatal("allowed PPPoE IPv4 destination was rejected")
	}
	disallowed := pppoeIPv4Frame([4]byte{198, 51, 100, 1}, [4]byte{10, 21, 0, 2})
	if tapFrameAllowed(disallowed, guard, false) {
		t.Fatal("PPPoE IPv4 destination bypassed guard")
	}
	if tapFrameAllowed(disallowed[:20], guard, false) {
		t.Fatal("truncated PPPoE session was accepted")
	}
}

func TestTapFrameAllowsPPPoEDiscoveryWithIPGuard(t *testing.T) {
	guard := fastpath.AddressGuard{IPv4Prefixes: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/24")}}
	frame := ethernetHeader(0x8863)
	frame = append(frame, 0x11, 0x09, 0, 0, 0, 0)
	if !tapFrameAllowed(frame, guard, true) {
		t.Fatal("PPPoE discovery has no IP address and should remain transparent")
	}
}

func TestAddressGuardRejectsUnlistedIPFamily(t *testing.T) {
	ipv4Guard := fastpath.AddressGuard{IPv4Prefixes: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/24")}}
	ipv6Packet := make([]byte, 40)
	ipv6Packet[0] = 0x60
	if tunFrameAllowed(ipv6Packet, ipv4Guard, true) {
		t.Fatal("IPv6 TUN packet bypassed an IPv4-only allow list")
	}
	ipv6Frame := append(ethernetHeader(0x86dd), ipv6Packet...)
	if tapFrameAllowed(ipv6Frame, ipv4Guard, true) {
		t.Fatal("IPv6 TAP frame bypassed an IPv4-only allow list")
	}

	ipv6Guard := fastpath.AddressGuard{IPv6Prefixes: []netip.Prefix{netip.MustParsePrefix("2001:db8::/64")}}
	ipv4Packet := ipv4Header([4]byte{10, 20, 0, 2}, [4]byte{10, 20, 0, 3})
	if tunFrameAllowed(ipv4Packet, ipv6Guard, true) {
		t.Fatal("IPv4 TUN packet bypassed an IPv6-only allow list")
	}
	ipv4Frame := append(ethernetHeader(0x0800), ipv4Packet...)
	if tapFrameAllowed(ipv4Frame, ipv6Guard, true) {
		t.Fatal("IPv4 TAP frame bypassed an IPv6-only allow list")
	}
	arpFrame := append(ethernetHeader(0x0806), make([]byte, 28)...)
	if tapFrameAllowed(arpFrame, ipv6Guard, true) {
		t.Fatal("ARP frame bypassed an IPv6-only allow list")
	}
}

func ethernetIPv4Frame(tags []uint16, source, destination [4]byte) []byte {
	frame := make([]byte, 0, 14+len(tags)*4+20)
	frame = append(frame, []byte{0x02, 0, 0, 0, 0, 2, 0x02, 0, 0, 0, 0, 1}...)
	if len(tags) == 0 {
		frame = append(frame, 0x08, 0x00)
	} else {
		frame = append(frame, byte(tags[0]>>8), byte(tags[0]))
		for index := range tags {
			next := uint16(0x0800)
			if index+1 < len(tags) {
				next = tags[index+1]
			}
			frame = append(frame, 0, byte(index+1), byte(next>>8), byte(next))
		}
	}
	return append(frame, ipv4Header(source, destination)...)
}

func pppoeIPv4Frame(source, destination [4]byte) []byte {
	frame := ethernetHeader(0x8864)
	packet := ipv4Header(source, destination)
	length := len(packet) + 2
	frame = append(frame, 0x11, 0, 0, 1, byte(length>>8), byte(length), 0, 0x21)
	return append(frame, packet...)
}

func ethernetHeader(etherType uint16) []byte {
	return []byte{0x02, 0, 0, 0, 0, 2, 0x02, 0, 0, 0, 0, 1, byte(etherType >> 8), byte(etherType)}
}

func ipv4Header(source, destination [4]byte) []byte {
	packet := make([]byte, 20)
	packet[0] = 0x45
	packet[2], packet[3] = 0, 20
	packet[8], packet[9] = 64, 17
	copy(packet[12:16], source[:])
	copy(packet[16:20], destination[:])
	return packet
}
