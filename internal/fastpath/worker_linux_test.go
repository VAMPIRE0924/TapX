//go:build linux && cgo

package fastpath

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

func TestStartUDPPipeSegmentsAndReassemblesOutOfOrder(t *testing.T) {
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
	vkeyHeaderSize := 8 + len(key)

	const maxDatagramPayload = 128
	worker, err := StartUDPPipe(UDPConfig{
		TUNFD:              tun[0],
		UDPFD:              int(udpFile.Fd()),
		FrameKind:          FrameTUN,
		MaxFrameSize:       2048,
		MaxDatagramPayload: maxDatagramPayload,
		PeerMode:           UDPPeerFixed,
		Peer:               netip.MustParseAddrPort(peerAddr.String()),
		VKey:               key,
	})
	if err != nil {
		t.Fatalf("StartUDPPipe: %v", err)
	}

	outgoing := make([]byte, 300)
	for i := range outgoing {
		outgoing[i] = byte(i)
	}
	outgoing[0] = 0x45
	if _, err := syscall.Write(tun[1], outgoing); err != nil {
		t.Fatalf("write tun: %v", err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	reassembled := make([]byte, len(outgoing))
	const fragmentPayload = maxDatagramPayload - 20 - 15
	const fragmentCount = 4
	for wantIndex := 0; wantIndex < fragmentCount; wantIndex++ {
		wire := make([]byte, maxDatagramPayload)
		n, _, err := peer.ReadFromUDP(wire)
		if err != nil {
			t.Fatalf("read segment %d: %v", wantIndex, err)
		}
		wire = wire[:n]
		if !bytes.Equal(wire[:vkeyHeaderSize], vkeyPayload(key, nil)) {
			t.Fatalf("segment %d vKey header = %x", wantIndex, wire[:vkeyHeaderSize])
		}
		header := wire[vkeyHeaderSize:]
		if string(header[:4]) != "TXS1" || int(binary.BigEndian.Uint16(header[10:12])) != wantIndex ||
			binary.BigEndian.Uint16(header[12:14]) != fragmentCount ||
			binary.BigEndian.Uint16(header[14:16]) != fragmentPayload {
			t.Fatalf("segment %d header = %x", wantIndex, header[:20])
		}
		fragmentLen := int(binary.BigEndian.Uint16(header[16:18]))
		copy(reassembled[wantIndex*fragmentPayload:], header[20:20+fragmentLen])
	}
	if !bytes.Equal(reassembled, outgoing) {
		t.Fatal("outgoing segments did not reconstruct the original frame")
	}

	incoming := make([]byte, 300)
	for i := range incoming {
		incoming[i] = byte(255 - i)
	}
	incoming[0] = 0x60
	// The reverse path may have a larger confirmed datagram ceiling. Receiving
	// must not be truncated to this worker's independent send ceiling.
	segments := testSegmentPayloads(77, incoming, 200-vkeyHeaderSize)
	udpAddr := udp.LocalAddr().(*net.UDPAddr)
	for _, index := range []int{1, 0, 0} {
		if _, err := peer.WriteToUDP(vkeyPayload(key, segments[index]), udpAddr); err != nil {
			t.Fatalf("write segment %d: %v", index, err)
		}
	}
	pollFDs := []unix.PollFd{{Fd: int32(tun[1]), Events: unix.POLLIN}}
	if n, err := unix.Poll(pollFDs, 1000); err != nil || n != 1 {
		t.Fatalf("poll reassembled frame = %d, %v", n, err)
	}
	buf := make([]byte, 2048)
	n, err := syscall.Read(tun[1], buf)
	if err != nil {
		t.Fatalf("read reassembled frame: %v", err)
	}
	if !bytes.Equal(buf[:n], incoming) {
		t.Fatal("reassembled incoming frame does not match")
	}
	expectNoUnixData(t, tun[1])

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.TXPackets != 1 || snap.TXBytes != uint64(len(outgoing)) ||
		snap.RXPackets != 1 || snap.RXBytes != uint64(len(incoming)) {
		t.Fatalf("counters = %+v, want original-frame accounting", snap)
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
	if _, err := syscall.Write(tun[1], ipv6Packet("2001:db8::2", "2001:db8::3")); err != nil {
		t.Fatalf("write unlisted IPv6 family: %v", err)
	}
	expectNoUDPData(t, peer)

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 3 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want three guard drops and one packet each direction", snap)
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
	if _, err := syscall.Write(tun[1], ipv4Packet(10, 0, 0, 2, 10, 0, 0, 3)); err != nil {
		t.Fatalf("write unlisted IPv4 family: %v", err)
	}
	expectNoUDPData(t, peer)

	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	snap := worker.Counters()
	if snap.DropsGuard != 3 || snap.TXPackets != 1 || snap.RXPackets != 1 {
		t.Fatalf("counters = %+v, want three guard drops and one packet each direction", snap)
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

func TestStartTCPPipeBackpressurePreservesFrames(t *testing.T) {
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
	if err := syscall.SetsockoptInt(tcp[0], syscall.SOL_SOCKET, syscall.SO_SNDBUF, 4096); err != nil {
		t.Fatalf("set tcp send buffer: %v", err)
	}

	worker, err := StartTCPPipe(TCPConfig{
		TUNFD: tun[0], TCPFD: tcp[0], FrameKind: FrameTUN,
		MaxFrameSize: 2048, LengthMode: TCPLength16,
	})
	if err != nil {
		t.Fatalf("StartTCPPipe: %v", err)
	}
	defer worker.Stop()

	const frameCount = 1024
	sendDone := make(chan error, 1)
	go func() {
		payload := make([]byte, 1024)
		payload[0] = 0x45
		for sequence := 0; sequence < frameCount; sequence++ {
			binary.BigEndian.PutUint32(payload[4:8], uint32(sequence))
			if _, err := syscall.Write(tun[1], payload); err != nil {
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()

	// Let the small TCP send buffer fill before the receiver starts draining it.
	time.Sleep(50 * time.Millisecond)
	receiveDone := make(chan error, 1)
	go func() {
		stream := make([]byte, 0, 128*1024)
		buffer := make([]byte, 32*1024)
		next := 0
		for next < frameCount {
			n, err := syscall.Read(tcp[1], buffer)
			if err != nil {
				receiveDone <- err
				return
			}
			stream = append(stream, buffer[:n]...)
			for len(stream) >= 2 {
				frameSize := int(binary.BigEndian.Uint16(stream[:2]))
				if len(stream) < 2+frameSize {
					break
				}
				frame := stream[2 : 2+frameSize]
				if frameSize != 1024 || frame[0] != 0x45 || binary.BigEndian.Uint32(frame[4:8]) != uint32(next) {
					receiveDone <- fmt.Errorf("frame %d corrupted: size=%d prefix=%x sequence=%d", next, frameSize, frame[:8], binary.BigEndian.Uint32(frame[4:8]))
					return
				}
				next++
				stream = stream[2+frameSize:]
			}
		}
		receiveDone <- nil
	}()

	for name, done := range map[string]<-chan error{"sender": sendDone, "receiver": receiveDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s failed: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s timed out", name)
		}
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

func testSegmentPayloads(sequence uint32, frame []byte, maxDatagramPayload int) [][]byte {
	const headerSize = 20
	fragmentPayload := maxDatagramPayload - headerSize
	fragmentCount := (len(frame) + fragmentPayload - 1) / fragmentPayload
	segments := make([][]byte, 0, fragmentCount)
	for index := 0; index < fragmentCount; index++ {
		offset := index * fragmentPayload
		fragmentLen := len(frame) - offset
		if fragmentLen > fragmentPayload {
			fragmentLen = fragmentPayload
		}
		segment := make([]byte, headerSize+fragmentLen)
		copy(segment[:4], "TXS1")
		binary.BigEndian.PutUint32(segment[4:8], sequence)
		binary.BigEndian.PutUint16(segment[8:10], uint16(len(frame)))
		binary.BigEndian.PutUint16(segment[10:12], uint16(index))
		binary.BigEndian.PutUint16(segment[12:14], uint16(fragmentCount))
		binary.BigEndian.PutUint16(segment[14:16], uint16(fragmentPayload))
		binary.BigEndian.PutUint16(segment[16:18], uint16(fragmentLen))
		copy(segment[headerSize:], frame[offset:offset+fragmentLen])
		segments = append(segments, segment)
	}
	return segments
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
