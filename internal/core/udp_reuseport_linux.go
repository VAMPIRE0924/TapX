//go:build linux

package core

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
)

const (
	vkeyWireMagic       = uint32(0x54585631) // TXV1
	maxSocketBPFProgram = 4096
)

type udpVKeySocketRoute struct {
	VKey        string
	SocketIndex uint32
}

type udpReuseportGroup struct {
	Dispatch config.RuntimeUDPDispatch
	Local    netip.AddrPort
	fd       int
}

func startUDPReuseportGroup(dispatch config.RuntimeUDPDispatch, prototype config.RuntimeUDPPipe) (*udpReuseportGroup, error) {
	mode, err := fastpath.PeerModeFromModel(prototype.PeerMode)
	if err != nil {
		return nil, err
	}
	peer, _, err := peerForPipe(prototype, mode)
	if err != nil {
		return nil, err
	}
	prototype.ReusePort = true
	routes := make([]udpVKeySocketRoute, 0, len(dispatch.Routes))
	for _, route := range dispatch.Routes {
		routes = append(routes, udpVKeySocketRoute{VKey: route.VKeyValue, SocketIndex: route.SocketIndex})
	}
	fd, local, err := openUDPSocketWithHook(prototype, peer, func(fd int) error {
		if err := attachDropAllSocketFilter(fd); err != nil {
			return err
		}
		return attachUDPReuseportVKeyProgram(fd, routes, dispatch.FallbackSocketIndex, 0)
	})
	if err != nil {
		return nil, err
	}
	return &udpReuseportGroup{Dispatch: dispatch, Local: local, fd: fd}, nil
}

func (g *udpReuseportGroup) Close() error {
	if g == nil || g.fd < 0 {
		return nil
	}
	err := unix.Close(g.fd)
	g.fd = -1
	return err
}

func attachUDPReuseportVKeyProgram(fd int, routes []udpVKeySocketRoute, fallbackIndex, sinkIndex uint32) error {
	filters, err := buildUDPReuseportVKeyProgram(routes, fallbackIndex, sinkIndex)
	if err != nil {
		return err
	}
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_REUSEPORT_CBPF, &program); err != nil {
		return fmt.Errorf("core: attach UDP vKey reuseport classifier: %w", err)
	}
	return nil
}

func attachDropAllSocketFilter(fd int) error {
	filters := []unix.SockFilter{{Code: unix.BPF_RET | unix.BPF_K, K: 0}}
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &program); err != nil {
		return fmt.Errorf("core: attach UDP sink filter: %w", err)
	}
	return nil
}

func buildUDPReuseportVKeyProgram(routes []udpVKeySocketRoute, fallbackIndex, sinkIndex uint32) ([]unix.SockFilter, error) {
	filters := make([]unix.SockFilter, 0, 16+len(routes)*12)
	for _, route := range routes {
		key := []byte(route.VKey)
		if len(key) == 0 {
			return nil, fmt.Errorf("core: UDP reuseport route has an empty vKey")
		}
		if len(key) > 0xffff {
			return nil, fmt.Errorf("core: UDP reuseport vKey is too long: %d", len(key))
		}
		jumps := make([]int, 0, 3+len(key)/4)
		appendSocketBPFEqual(&filters, unix.BPF_W, 0, vkeyWireMagic, &jumps)
		appendSocketBPFEqual(&filters, unix.BPF_H, 4, uint32(len(key)), &jumps)
		offset := 0
		for len(key)-offset >= 4 {
			appendSocketBPFEqual(&filters, unix.BPF_W, uint32(8+offset), binary.BigEndian.Uint32(key[offset:offset+4]), &jumps)
			offset += 4
		}
		if len(key)-offset >= 2 {
			appendSocketBPFEqual(&filters, unix.BPF_H, uint32(8+offset), uint32(binary.BigEndian.Uint16(key[offset:offset+2])), &jumps)
			offset += 2
		}
		if len(key)-offset == 1 {
			appendSocketBPFEqual(&filters, unix.BPF_B, uint32(8+offset), uint32(key[offset]), &jumps)
		}
		filters = append(filters, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: route.SocketIndex})
		next := len(filters)
		for _, jump := range jumps {
			filters[jump].K = uint32(next - jump - 1)
		}
	}

	// A valid but unknown TXV1 header must never fall through to the no-vKey
	// route. Short packets and ordinary frames use the configured fallback.
	filters = append(filters,
		unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 0, Jf: 1, K: vkeyWireMagic},
		unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: sinkIndex},
		unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: fallbackIndex},
	)
	if len(filters) > maxSocketBPFProgram {
		return nil, fmt.Errorf("core: UDP vKey classifier needs %d instructions, maximum is %d", len(filters), maxSocketBPFProgram)
	}
	return filters, nil
}

func appendSocketBPFEqual(filters *[]unix.SockFilter, size uint16, offset, value uint32, jumps *[]int) {
	*filters = append(*filters,
		unix.SockFilter{Code: unix.BPF_LD | size | unix.BPF_ABS, K: offset},
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, K: value},
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JA},
	)
	*jumps = append(*jumps, len(*filters)-1)
}
