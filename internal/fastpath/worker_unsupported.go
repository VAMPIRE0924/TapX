//go:build !linux || !cgo

package fastpath

import (
	"errors"
	"net/netip"

	"tapx/internal/model"
)

type FrameKind uint32

const (
	FrameTUN FrameKind = 1
	FrameTAP FrameKind = 2
)

type UDPPeerMode uint32

const (
	UDPPeerAny   UDPPeerMode = 0
	UDPPeerFixed UDPPeerMode = 1
	UDPPeerLearn UDPPeerMode = 2
)

type TCPLengthMode uint32

const (
	TCPLength16 TCPLengthMode = 16
	TCPLength32 TCPLengthMode = 32
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

type Counters struct{}

func NewCounters() *Counters { return &Counters{} }
func (c *Counters) Reset()   {}
func (c *Counters) Snapshot() CountersSnapshot {
	return CountersSnapshot{}
}

type UDPConfig struct {
	TUNFD              int
	UDPFD              int
	FrameKind          FrameKind
	MaxFrameSize       uint32
	MaxDatagramPayload uint32
	PeerMode           UDPPeerMode
	Peer               netip.AddrPort
	VKey               []byte
	AddressGuard       AddressGuard
	Counters           *Counters
}

type TCPConfig struct {
	TUNFD        int
	TCPFD        int
	FrameKind    FrameKind
	MaxFrameSize uint32
	LengthMode   TCPLengthMode
	VKey         []byte
	AddressGuard AddressGuard
	Counters     *Counters
}

type Worker struct{}

func ABI() uint32 { return 0 }

func FrameKindFromDevice(deviceType model.DeviceType) (FrameKind, error) {
	switch deviceType {
	case model.DeviceTUN:
		return FrameTUN, nil
	case model.DeviceTAP:
		return FrameTAP, nil
	default:
		return 0, errors.New("fastpath: unsupported device type")
	}
}

func PeerModeFromModel(mode model.UDPPeerMode) (UDPPeerMode, error) {
	switch mode {
	case "", model.UDPPeerAny:
		return UDPPeerAny, nil
	case model.UDPPeerFixed:
		return UDPPeerFixed, nil
	case model.UDPPeerLearn:
		return UDPPeerLearn, nil
	default:
		return 0, errors.New("fastpath: unsupported udp peer mode")
	}
}

func TCPLengthModeFromModel(mode model.TCPLengthMode) (TCPLengthMode, error) {
	switch mode {
	case "", model.TCPLength16:
		return TCPLength16, nil
	case model.TCPLength32:
		return TCPLength32, nil
	default:
		return 0, errors.New("fastpath: unsupported tcp length mode")
	}
}

func StartUDPPipe(UDPConfig) (*Worker, error) {
	return nil, errors.New("fastpath: linux cgo support is required")
}

func StartTCPPipe(TCPConfig) (*Worker, error) {
	return nil, errors.New("fastpath: linux cgo support is required")
}

func (w *Worker) Stop() error { return nil }
func (w *Worker) Counters() CountersSnapshot {
	return CountersSnapshot{}
}
