//go:build linux && cgo

package fastpath

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStartUDPPipeBidirectional(t *testing.T) {
	tun, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(tun[0])
	defer syscall.Close(tun[1])

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udp.Close()
	udpFile, err := udp.File()
	if err != nil {
		t.Fatalf("udp file: %v", err)
	}
	defer udpFile.Close()

	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen peer: %v", err)
	}
	defer peer.Close()
	peerAddr := peer.LocalAddr().(*net.UDPAddr)

	worker, err := StartUDPPipe(UDPConfig{
		TUNFD:        tun[0],
		UDPFD:        int(udpFile.Fd()),
		FrameKind:    FrameTUN,
		MaxFrameSize: 2048,
		PeerMode:     UDPPeerFixed,
		Peer:         netip.MustParseAddrPort(peerAddr.String()),
	})
	if err != nil {
		t.Fatalf("StartUDPPipe: %v", err)
	}

	outgoing := []byte{0x45, 0x00, 0x00, 0x14}
	if _, err := syscall.Write(tun[1], outgoing); err != nil {
		t.Fatalf("write tun: %v", err)
	}
	buf := make([]byte, 64)
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read peer: %v", err)
	}
	if !bytes.Equal(buf[:n], outgoing) {
		t.Fatalf("peer payload = %v, want %v", buf[:n], outgoing)
	}

	incoming := []byte{0x60, 0x00, 0x00, 0x00}
	udpAddr := udp.LocalAddr().(*net.UDPAddr)
	if _, err := peer.WriteToUDP(incoming, udpAddr); err != nil {
		t.Fatalf("write peer: %v", err)
	}
	n, err = syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read tun: %v", err)
	}
	if !bytes.Equal(buf[:n], incoming) {
		t.Fatalf("tun payload = %v, want %v", buf[:n], incoming)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want one packet each direction", snap)
	}
}

func TestStartUDPPipeVKey(t *testing.T) {
	tun, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(tun[0])
	defer syscall.Close(tun[1])

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udp.Close()
	udpFile, err := udp.File()
	if err != nil {
		t.Fatalf("udp file: %v", err)
	}
	defer udpFile.Close()

	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen peer: %v", err)
	}
	defer peer.Close()
	peerAddr := peer.LocalAddr().(*net.UDPAddr)
	key := []byte("vk-demo")

	worker, err := StartUDPPipe(UDPConfig{
		TUNFD:        tun[0],
		UDPFD:        int(udpFile.Fd()),
		FrameKind:    FrameTUN,
		MaxFrameSize: 2048,
		PeerMode:     UDPPeerFixed,
		Peer:         netip.MustParseAddrPort(peerAddr.String()),
		VKey:         key,
	})
	if err != nil {
		t.Fatalf("StartUDPPipe: %v", err)
	}

	outgoing := ipv4Packet(10, 0, 0, 2, 10, 0, 0, 3)
	if _, err := syscall.Write(tun[1], outgoing); err != nil {
		t.Fatalf("write tun: %v", err)
	}
	buf := make([]byte, 128)
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read peer: %v", err)
	}
	wantWire := vkeyPayload(key, outgoing)
	if !bytes.Equal(buf[:n], wantWire) {
		t.Fatalf("wire payload = %v, want %v", buf[:n], wantWire)
	}

	udpAddr := udp.LocalAddr().(*net.UDPAddr)
	if _, err := peer.WriteToUDP(outgoing, udpAddr); err != nil {
		t.Fatalf("write missing vkey: %v", err)
	}
	expectNoUnixData(t, tun[1])

	if _, err := peer.WriteToUDP(wantWire, udpAddr); err != nil {
		t.Fatalf("write vkey payload: %v", err)
	}
	n, err = syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read tun: %v", err)
	}
	if !bytes.Equal(buf[:n], outgoing) {
		t.Fatalf("tun payload = %v, want %v", buf[:n], outgoing)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 1 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want one guard drop and one packet each direction", snap)
	}
}

func TestStartUDPPipeTUNIPv4AddressGuard(t *testing.T) {
	tun, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(tun[0])
	defer syscall.Close(tun[1])

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udp.Close()
	udpFile, err := udp.File()
	if err != nil {
		t.Fatalf("udp file: %v", err)
	}
	defer udpFile.Close()

	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen peer: %v", err)
	}
	defer peer.Close()
	peerAddr := peer.LocalAddr().(*net.UDPAddr)

	worker, err := StartUDPPipe(UDPConfig{
		TUNFD:        tun[0],
		UDPFD:        int(udpFile.Fd()),
		FrameKind:    FrameTUN,
		MaxFrameSize: 2048,
		PeerMode:     UDPPeerFixed,
		Peer:         netip.MustParseAddrPort(peerAddr.String()),
		AddressGuard: AddressGuard{
			IPv4Prefixes: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
		},
	})
	if err != nil {
		t.Fatalf("StartUDPPipe: %v", err)
	}

	if _, err := syscall.Write(tun[1], ipv4Packet(10, 0, 1, 2, 10, 0, 0, 3)); err != nil {
		t.Fatalf("write disallowed tun: %v", err)
	}
	expectNoUDPData(t, peer)

	allowedOut := ipv4Packet(10, 0, 0, 2, 10, 0, 1, 3)
	if _, err := syscall.Write(tun[1], allowedOut); err != nil {
		t.Fatalf("write allowed tun: %v", err)
	}
	buf := make([]byte, 64)
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read peer: %v", err)
	}
	if !bytes.Equal(buf[:n], allowedOut) {
		t.Fatalf("allowed peer payload = %v, want %v", buf[:n], allowedOut)
	}

	udpAddr := udp.LocalAddr().(*net.UDPAddr)
	if _, err := peer.WriteToUDP(ipv4Packet(10, 0, 1, 3, 10, 0, 1, 2), udpAddr); err != nil {
		t.Fatalf("write disallowed peer: %v", err)
	}
	expectNoUnixData(t, tun[1])

	allowedIn := ipv4Packet(10, 0, 1, 3, 10, 0, 0, 2)
	if _, err := peer.WriteToUDP(allowedIn, udpAddr); err != nil {
		t.Fatalf("write allowed peer: %v", err)
	}
	n, err = syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read tun: %v", err)
	}
	if !bytes.Equal(buf[:n], allowedIn) {
		t.Fatalf("allowed tun payload = %v, want %v", buf[:n], allowedIn)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 2 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want two guard drops and one packet each direction", snap)
	}
}

func TestStartUDPPipeTUNIPv6AddressGuard(t *testing.T) {
	tun, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(tun[0])
	defer syscall.Close(tun[1])

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udp.Close()
	udpFile, err := udp.File()
	if err != nil {
		t.Fatalf("udp file: %v", err)
	}
	defer udpFile.Close()

	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen peer: %v", err)
	}
	defer peer.Close()
	peerAddr := peer.LocalAddr().(*net.UDPAddr)

	worker, err := StartUDPPipe(UDPConfig{
		TUNFD:        tun[0],
		UDPFD:        int(udpFile.Fd()),
		FrameKind:    FrameTUN,
		MaxFrameSize: 2048,
		PeerMode:     UDPPeerFixed,
		Peer:         netip.MustParseAddrPort(peerAddr.String()),
		AddressGuard: AddressGuard{
			IPv6Prefixes: []netip.Prefix{netip.MustParsePrefix("2001:db8::/64")},
		},
	})
	if err != nil {
		t.Fatalf("StartUDPPipe: %v", err)
	}

	if _, err := syscall.Write(tun[1], ipv6Packet("2001:db9::2", "2001:db8::3")); err != nil {
		t.Fatalf("write disallowed tun: %v", err)
	}
	expectNoUDPData(t, peer)

	allowedOut := ipv6Packet("2001:db8::2", "2001:db9::3")
	if _, err := syscall.Write(tun[1], allowedOut); err != nil {
		t.Fatalf("write allowed tun: %v", err)
	}
	buf := make([]byte, 80)
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read peer: %v", err)
	}
	if !bytes.Equal(buf[:n], allowedOut) {
		t.Fatalf("allowed peer payload = %v, want %v", buf[:n], allowedOut)
	}

	udpAddr := udp.LocalAddr().(*net.UDPAddr)
	if _, err := peer.WriteToUDP(ipv6Packet("2001:db9::9", "2001:db9::2"), udpAddr); err != nil {
		t.Fatalf("write disallowed peer: %v", err)
	}
	expectNoUnixData(t, tun[1])

	allowedIn := ipv6Packet("2001:db9::9", "2001:db8::2")
	if _, err := peer.WriteToUDP(allowedIn, udpAddr); err != nil {
		t.Fatalf("write allowed peer: %v", err)
	}
	n, err = syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read tun: %v", err)
	}
	if !bytes.Equal(buf[:n], allowedIn) {
		t.Fatalf("allowed tun payload = %v, want %v", buf[:n], allowedIn)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 2 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want two guard drops and one packet each direction", snap)
	}
}

func TestStartUDPPipeTAPMACAddressGuard(t *testing.T) {
	tap, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair tap: %v", err)
	}
	defer syscall.Close(tap[0])
	defer syscall.Close(tap[1])

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udp.Close()
	udpFile, err := udp.File()
	if err != nil {
		t.Fatalf("udp file: %v", err)
	}
	defer udpFile.Close()

	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen peer: %v", err)
	}
	defer peer.Close()
	peerAddr := peer.LocalAddr().(*net.UDPAddr)

	allowedMAC := [6]byte{0x02, 0, 0, 0, 0, 1}
	otherMAC := [6]byte{0x02, 0, 0, 0, 0, 2}
	peerMAC := [6]byte{0x02, 0xaa, 0, 0, 0, 1}

	worker, err := StartUDPPipe(UDPConfig{
		TUNFD:        tap[0],
		UDPFD:        int(udpFile.Fd()),
		FrameKind:    FrameTAP,
		MaxFrameSize: 2048,
		PeerMode:     UDPPeerFixed,
		Peer:         netip.MustParseAddrPort(peerAddr.String()),
		AddressGuard: AddressGuard{
			IPv4Prefixes: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
			MACs:         [][6]byte{allowedMAC},
		},
	})
	if err != nil {
		t.Fatalf("StartUDPPipe: %v", err)
	}

	if _, err := syscall.Write(tap[1], ethernetIPv4Frame(peerMAC, otherMAC, 10, 0, 0, 2, 10, 0, 0, 3)); err != nil {
		t.Fatalf("write bad source mac: %v", err)
	}
	expectNoUDPData(t, peer)

	allowedOut := ethernetIPv4Frame(peerMAC, allowedMAC, 10, 0, 0, 2, 10, 0, 1, 3)
	if _, err := syscall.Write(tap[1], allowedOut); err != nil {
		t.Fatalf("write allowed tap: %v", err)
	}
	buf := make([]byte, 64)
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read peer: %v", err)
	}
	if !bytes.Equal(buf[:n], allowedOut) {
		t.Fatalf("allowed TAP payload = %v, want %v", buf[:n], allowedOut)
	}

	udpAddr := udp.LocalAddr().(*net.UDPAddr)
	if _, err := peer.WriteToUDP(ethernetIPv4Frame(otherMAC, peerMAC, 10, 0, 1, 3, 10, 0, 0, 2), udpAddr); err != nil {
		t.Fatalf("write bad destination mac: %v", err)
	}
	expectNoUnixData(t, tap[1])

	allowedIn := ethernetIPv4Frame(allowedMAC, peerMAC, 10, 0, 1, 3, 10, 0, 0, 2)
	if _, err := peer.WriteToUDP(allowedIn, udpAddr); err != nil {
		t.Fatalf("write allowed peer: %v", err)
	}
	n, err = syscall.Read(tap[1], buf)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	if !bytes.Equal(buf[:n], allowedIn) {
		t.Fatalf("allowed TAP inbound = %v, want %v", buf[:n], allowedIn)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 2 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want two guard drops and one packet each direction", snap)
	}
}

func TestStartTCPPipeBidirectional(t *testing.T) {
	tun, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair tun: %v", err)
	}
	defer syscall.Close(tun[0])
	defer syscall.Close(tun[1])

	tcp, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair tcp: %v", err)
	}
	defer syscall.Close(tcp[0])
	defer syscall.Close(tcp[1])

	worker, err := StartTCPPipe(TCPConfig{
		TUNFD:        tun[0],
		TCPFD:        tcp[0],
		FrameKind:    FrameTUN,
		MaxFrameSize: 2048,
		LengthMode:   TCPLength16,
	})
	if err != nil {
		t.Fatalf("StartTCPPipe: %v", err)
	}

	outgoing := []byte{0x45, 0x00, 0x00, 0x14}
	if _, err := syscall.Write(tun[1], outgoing); err != nil {
		t.Fatalf("write tun: %v", err)
	}
	buf := make([]byte, 64)
	n, err := syscall.Read(tcp[1], buf)
	if err != nil {
		t.Fatalf("read tcp: %v", err)
	}
	if n != len(outgoing)+2 {
		t.Fatalf("tcp frame length = %d, want %d", n, len(outgoing)+2)
	}
	if got := binary.BigEndian.Uint16(buf[:2]); got != uint16(len(outgoing)) {
		t.Fatalf("tcp frame header = %d, want %d", got, len(outgoing))
	}
	if !bytes.Equal(buf[2:n], outgoing) {
		t.Fatalf("tcp payload = %v, want %v", buf[2:n], outgoing)
	}

	incoming := []byte{0x60, 0x00, 0x00, 0x00}
	var frame [6]byte
	binary.BigEndian.PutUint16(frame[:2], uint16(len(incoming)))
	copy(frame[2:], incoming)
	if _, err := syscall.Write(tcp[1], frame[:]); err != nil {
		t.Fatalf("write tcp: %v", err)
	}
	n, err = syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read tun: %v", err)
	}
	if !bytes.Equal(buf[:n], incoming) {
		t.Fatalf("tun payload = %v, want %v", buf[:n], incoming)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want one packet each direction", snap)
	}
}

func TestStartTCPPipeVKey(t *testing.T) {
	tun, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair tun: %v", err)
	}
	defer syscall.Close(tun[0])
	defer syscall.Close(tun[1])

	tcp, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair tcp: %v", err)
	}
	defer syscall.Close(tcp[0])
	defer syscall.Close(tcp[1])

	key := []byte("tcp-vk")
	worker, err := StartTCPPipe(TCPConfig{
		TUNFD:        tun[0],
		TCPFD:        tcp[0],
		FrameKind:    FrameTUN,
		MaxFrameSize: 2048,
		LengthMode:   TCPLength16,
		VKey:         key,
	})
	if err != nil {
		t.Fatalf("StartTCPPipe: %v", err)
	}

	outgoing := []byte{0x45, 0x00, 0x00, 0x14}
	if _, err := syscall.Write(tun[1], outgoing); err != nil {
		t.Fatalf("write tun: %v", err)
	}
	buf := make([]byte, 128)
	n, err := syscall.Read(tcp[1], buf)
	if err != nil {
		t.Fatalf("read tcp: %v", err)
	}
	wantPayload := vkeyPayload(key, outgoing)
	if n != len(wantPayload)+2 {
		t.Fatalf("tcp frame length = %d, want %d", n, len(wantPayload)+2)
	}
	if got := binary.BigEndian.Uint16(buf[:2]); got != uint16(len(wantPayload)) {
		t.Fatalf("tcp length = %d, want %d", got, len(wantPayload))
	}
	if !bytes.Equal(buf[2:n], wantPayload) {
		t.Fatalf("tcp wire payload = %v, want %v", buf[2:n], wantPayload)
	}

	var bad [6]byte
	binary.BigEndian.PutUint16(bad[:2], uint16(len(outgoing)))
	copy(bad[2:], outgoing)
	if _, err := syscall.Write(tcp[1], bad[:]); err != nil {
		t.Fatalf("write missing vkey: %v", err)
	}
	expectNoUnixData(t, tun[1])

	wire := make([]byte, len(wantPayload)+2)
	binary.BigEndian.PutUint16(wire[:2], uint16(len(wantPayload)))
	copy(wire[2:], wantPayload)
	if _, err := syscall.Write(tcp[1], wire); err != nil {
		t.Fatalf("write vkey frame: %v", err)
	}
	n, err = syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read tun: %v", err)
	}
	if !bytes.Equal(buf[:n], outgoing) {
		t.Fatalf("tun payload = %v, want %v", buf[:n], outgoing)
	}

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 1 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want one guard drop and one packet each direction", snap)
	}
}

func ethernetIPv4Frame(dst, src [6]byte, src0, src1, src2, src3, dst0, dst1, dst2, dst3 byte) []byte {
	frame := make([]byte, 34)
	copy(frame[:6], dst[:])
	copy(frame[6:12], src[:])
	frame[12] = 0x08
	frame[13] = 0x00
	copy(frame[14:], ipv4Packet(src0, src1, src2, src3, dst0, dst1, dst2, dst3))
	return frame
}

func vkeyPayload(key, payload []byte) []byte {
	out := make([]byte, 8+len(key)+len(payload))
	copy(out[:4], []byte("TXV1"))
	binary.BigEndian.PutUint16(out[4:6], uint16(len(key)))
	copy(out[8:], key)
	copy(out[8+len(key):], payload)
	return out
}

func ipv6Packet(src, dst string) []byte {
	packet := make([]byte, 40)
	packet[0] = 0x60
	packet[6] = 59
	packet[7] = 64
	srcAddr := netip.MustParseAddr(src).As16()
	dstAddr := netip.MustParseAddr(dst).As16()
	copy(packet[8:24], srcAddr[:])
	copy(packet[24:40], dstAddr[:])
	return packet
}

func ipv4Packet(src0, src1, src2, src3, dst0, dst1, dst2, dst3 byte) []byte {
	packet := make([]byte, 20)
	packet[0] = 0x45
	packet[2] = 0x00
	packet[3] = 0x14
	packet[8] = 64
	packet[9] = 59
	packet[12] = src0
	packet[13] = src1
	packet[14] = src2
	packet[15] = src3
	packet[16] = dst0
	packet[17] = dst1
	packet[18] = dst2
	packet[19] = dst3
	return packet
}

func expectNoUDPData(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var buf [64]byte
	_, _, err := conn.ReadFromUDP(buf[:])
	if err == nil {
		t.Fatal("unexpected UDP data")
	}
	if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("ReadFromUDP() error = %v, want timeout", err)
	}
}

func expectNoUnixData(t *testing.T, fd int) {
	t.Helper()
	pollFDs := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(pollFDs, 100)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if n != 0 {
		t.Fatal("unexpected unix socket data")
	}
}
