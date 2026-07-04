//go:build linux

package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/netapply"
	"tapx/internal/rawtcp"
	"tapx/internal/tuntap"
	"tapx/internal/xrayruntime"
)

type XrayPipeHandle struct {
	Pipe       config.RuntimeXrayPipe
	DeviceName string

	device   tuntap.Device
	netApply netapply.Handle
	counter  xrayPipeCounters

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	conn    net.Conn
	lastErr error
	active  bool
}

type xrayPipeCounters struct {
	rxPackets  atomic.Uint64
	txPackets  atomic.Uint64
	rxBytes    atomic.Uint64
	txBytes    atomic.Uint64
	dropsGuard atomic.Uint64
	dropsIO    atomic.Uint64
}

func startXrayPipe(pipe config.RuntimeXrayPipe, device config.RuntimeDevice, manager *xrayruntime.Manager) (*XrayPipeHandle, error) {
	if manager == nil {
		return nil, fmt.Errorf("core: xray manager is nil")
	}
	guard, err := fastpathAddressGuard(pipe.AddressGuard)
	if err != nil {
		return nil, err
	}
	frameKind, err := fastpath.FrameKindFromDevice(pipe.FrameKind)
	if err != nil {
		return nil, err
	}
	tunDevice, netHandle, err := openAppliedDevice(device, true)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	handle := &XrayPipeHandle{
		Pipe:       pipe,
		DeviceName: tunDevice.Name(),
		device:     tunDevice,
		netApply:   netHandle,
		cancel:     cancel,
	}

	switch pipe.EndpointKind {
	case "listener":
		tag := xrayruntime.FrameOutboundTag(pipe.EndpointID)
		if err := manager.RegisterStreamHandler(tag, func(streamCtx context.Context, conn net.Conn) {
			handle.runAcceptedStream(streamCtx, conn, frameKind, guard)
		}); err != nil {
			_ = handle.Close()
			return nil, err
		}
	case "connector":
		conn, err := manager.DialEmbeddedTCP(ctx, pipe.EndpointID, firstNonEmpty(pipe.Remote, "tapx.frame.local"), firstNonZero(pipe.Port, 1))
		if err != nil {
			_ = handle.Close()
			return nil, err
		}
		handle.startBridge(ctx, conn, frameKind, guard)
	default:
		_ = handle.Close()
		return nil, fmt.Errorf("core: xray pipe %s has unsupported endpoint kind %q", pipe.EndpointID, pipe.EndpointKind)
	}
	return handle, nil
}

func openAppliedDevice(device config.RuntimeDevice, nonBlock bool) (tuntap.Device, netapply.Handle, error) {
	tunDevice, err := tuntap.Open(tuntap.OpenOptions{
		Name:     device.IfName,
		Type:     device.Type,
		NonBlock: nonBlock,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("core: open %s %s: %w", device.Type, device.IfName, err)
	}
	netHandle, err := netapply.ApplyDevice(netapply.DeviceConfig{
		Type:     device.Type,
		IfName:   tunDevice.Name(),
		MTU:      device.MTU,
		MSSClamp: device.MSSClamp,
		IPv4CIDR: device.IPv4CIDR,
		IPv6CIDR: device.IPv6CIDR,
		Bridge: netapply.BridgeConfig{
			Enabled: device.Bridge.Enabled,
			Name:    device.Bridge.Name,
			IfName:  device.Bridge.IfName,
			MTU:     device.Bridge.MTU,
		},
		Routes: netapplyRoutes(device.Routes),
		DNS:    netapplyDNS(device.DNS),
	})
	if err != nil {
		_ = tunDevice.Close()
		return nil, nil, fmt.Errorf("core: apply device %s: %w", tunDevice.Name(), err)
	}
	return tunDevice, netHandle, nil
}

func (h *XrayPipeHandle) runAcceptedStream(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	if !h.markActive() {
		_ = conn.Close()
		return
	}
	defer h.clearActive()
	h.runBridge(ctx, conn, frameKind, guard)
}

func (h *XrayPipeHandle) startBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	h.mu.Lock()
	if h.done != nil {
		h.mu.Unlock()
		_ = conn.Close()
		return
	}
	done := make(chan struct{})
	h.done = done
	h.conn = conn
	h.mu.Unlock()

	go func() {
		defer close(done)
		h.bridge(ctx, conn, frameKind, guard)
		h.mu.Lock()
		if h.conn == conn {
			h.conn = nil
		}
		h.mu.Unlock()
	}()
}

func (h *XrayPipeHandle) runBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	done := make(chan struct{})
	h.mu.Lock()
	h.done = done
	h.conn = conn
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.done == done {
			h.done = nil
		}
		if h.conn == conn {
			h.conn = nil
		}
		h.mu.Unlock()
		close(done)
	}()
	h.bridge(ctx, conn, frameKind, guard)
}

func (h *XrayPipeHandle) bridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	defer conn.Close()
	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2)
	go func() { errc <- h.deviceToConn(bridgeCtx, conn, frameKind, guard) }()
	go func() { errc <- h.connToDevice(bridgeCtx, conn, frameKind, guard) }()
	err := <-errc
	cancel()
	_ = conn.Close()
	err2 := <-errc
	h.setBridgeErr(ctx, err)
	h.setBridgeErr(ctx, err2)
}

func (h *XrayPipeHandle) deviceToConn(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	buf := make([]byte, maxPositive(h.Pipe.MaxFrameSize, rawtcp.DefaultMaxFrameSize))
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := readDeviceFrame(ctx, h.device, buf)
		if err != nil {
			return err
		}
		frame := buf[:n]
		if !xrayFrameAllowed(frameKind, frame, guard) {
			h.counter.dropsGuard.Add(1)
			continue
		}
		if err := rawtcp.WriteFrame(conn, h.Pipe.LengthMode, frame); err != nil {
			h.counter.dropsIO.Add(1)
			return err
		}
		h.counter.txPackets.Add(1)
		h.counter.txBytes.Add(uint64(n))
	}
}

func (h *XrayPipeHandle) connToDevice(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := rawtcp.ReadFrame(conn, h.Pipe.LengthMode, maxPositive(h.Pipe.MaxFrameSize, rawtcp.DefaultMaxFrameSize))
		if err != nil {
			return err
		}
		if !xrayFrameAllowed(frameKind, frame, guard) {
			h.counter.dropsGuard.Add(1)
			continue
		}
		if _, err := h.device.Write(frame); err != nil {
			h.counter.dropsIO.Add(1)
			return err
		}
		h.counter.rxPackets.Add(1)
		h.counter.rxBytes.Add(uint64(len(frame)))
	}
}

func readDeviceFrame(ctx context.Context, device tuntap.Device, buf []byte) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		pollFDs := []unix.PollFd{{
			Fd:     int32(device.FD()),
			Events: unix.POLLIN,
		}}
		n, err := unix.Poll(pollFDs, int((100 * time.Millisecond).Milliseconds()))
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		revents := pollFDs[0].Revents
		if revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return 0, fmt.Errorf("core: xray pipe device poll failed: revents=0x%x", revents)
		}
		if revents&unix.POLLIN == 0 {
			continue
		}
		readN, err := device.Read(buf)
		if err == unix.EINTR || err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			continue
		}
		return readN, err
	}
}

func (h *XrayPipeHandle) setBridgeErr(ctx context.Context, err error) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	h.setErr(err)
}

func (h *XrayPipeHandle) Close() error {
	var firstErr error
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.mu.Lock()
	done := h.done
	conn := h.conn
	h.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if h.device != nil {
		if h.netApply != nil {
			if err := h.netApply.Rollback(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.netApply = nil
		}
		if err := h.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.device = nil
	}
	if done != nil {
		<-done
	}
	return firstErr
}

func (h *XrayPipeHandle) Counters() fastpath.CountersSnapshot {
	if h == nil {
		return fastpath.CountersSnapshot{}
	}
	return fastpath.CountersSnapshot{
		RXPackets:  h.counter.rxPackets.Load(),
		TXPackets:  h.counter.txPackets.Load(),
		RXBytes:    h.counter.rxBytes.Load(),
		TXBytes:    h.counter.txBytes.Load(),
		DropsGuard: h.counter.dropsGuard.Load(),
		DropsIO:    h.counter.dropsIO.Load(),
	}
}

func (h *XrayPipeHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErr
}

func (h *XrayPipeHandle) setErr(err error) {
	h.mu.Lock()
	if h.lastErr == nil {
		h.lastErr = err
	}
	h.mu.Unlock()
}

func (h *XrayPipeHandle) markActive() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active {
		return false
	}
	h.active = true
	return true
}

func (h *XrayPipeHandle) clearActive() {
	h.mu.Lock()
	h.active = false
	h.mu.Unlock()
}

func xrayFrameAllowed(kind fastpath.FrameKind, frame []byte, guard fastpath.AddressGuard) bool {
	if len(guard.IPv4Prefixes) == 0 && len(guard.IPv6Prefixes) == 0 && len(guard.MACs) == 0 {
		return true
	}
	switch kind {
	case fastpath.FrameTUN:
		return tunFrameAllowed(frame, guard)
	case fastpath.FrameTAP:
		return tapFrameAllowed(frame, guard)
	default:
		return false
	}
}

func tunFrameAllowed(frame []byte, guard fastpath.AddressGuard) bool {
	if len(frame) < 1 {
		return false
	}
	switch frame[0] >> 4 {
	case 4:
		if len(guard.IPv4Prefixes) == 0 {
			return true
		}
		if len(frame) < 20 {
			return false
		}
		return prefixContains4(guard.IPv4Prefixes, netip.AddrFrom4([4]byte{frame[12], frame[13], frame[14], frame[15]}))
	case 6:
		if len(guard.IPv6Prefixes) == 0 {
			return true
		}
		if len(frame) < 40 {
			return false
		}
		var src [16]byte
		copy(src[:], frame[8:24])
		return prefixContains6(guard.IPv6Prefixes, netip.AddrFrom16(src))
	default:
		return true
	}
}

func tapFrameAllowed(frame []byte, guard fastpath.AddressGuard) bool {
	if len(frame) < 14 {
		return false
	}
	if len(guard.MACs) > 0 {
		var src [6]byte
		copy(src[:], frame[6:12])
		if !macAllowed(guard.MACs, src) {
			return false
		}
	}
	etherType := uint16(frame[12])<<8 | uint16(frame[13])
	switch etherType {
	case 0x0800:
		if len(guard.IPv4Prefixes) == 0 {
			return true
		}
		if len(frame) < 34 {
			return false
		}
		return prefixContains4(guard.IPv4Prefixes, netip.AddrFrom4([4]byte{frame[26], frame[27], frame[28], frame[29]}))
	case 0x0806:
		if len(guard.IPv4Prefixes) == 0 {
			return true
		}
		if len(frame) < 42 {
			return false
		}
		return prefixContains4(guard.IPv4Prefixes, netip.AddrFrom4([4]byte{frame[28], frame[29], frame[30], frame[31]}))
	case 0x86dd:
		if len(guard.IPv6Prefixes) == 0 {
			return true
		}
		if len(frame) < 54 {
			return false
		}
		var src [16]byte
		copy(src[:], frame[22:38])
		return prefixContains6(guard.IPv6Prefixes, netip.AddrFrom16(src))
	default:
		return true
	}
}

func prefixContains4(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func prefixContains6(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func macAllowed(allowed [][6]byte, mac [6]byte) bool {
	for _, item := range allowed {
		if item == mac {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func maxPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 1
}

func firstNonZero(values ...uint16) uint16 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
