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

	xbuf "github.com/xtls/xray-core/common/buf"
	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/linkdiag"
	"tapx/internal/model"
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
	shared   *xraySharedDevice
	owner    bool
	counter  xrayPipeCounters
	manager  *xrayruntime.Manager

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	conn    net.Conn
	lastErr error
	active  bool
}

type xraySharedDevice struct {
	device   tuntap.Device
	netApply netapply.Handle

	mu     sync.Mutex
	active bool
}

type xrayPipeCounters struct {
	rxPackets  atomic.Uint64
	txPackets  atomic.Uint64
	rxBytes    atomic.Uint64
	txBytes    atomic.Uint64
	dropsGuard atomic.Uint64
	dropsIO    atomic.Uint64
}

func startXrayPipeShared(pipe config.RuntimeXrayPipe, device config.RuntimeDevice, manager *xrayruntime.Manager, shared *xraySharedDevice) (*XrayPipeHandle, error) {
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
	owner := false
	if shared == nil {
		tunDevice, netHandle, err := openAppliedDevice(device, true)
		if err != nil {
			return nil, err
		}
		shared = &xraySharedDevice{device: tunDevice, netApply: netHandle}
		owner = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	handle := &XrayPipeHandle{
		Pipe:       pipe,
		DeviceName: shared.device.Name(),
		device:     shared.device,
		netApply:   shared.netApply,
		shared:     shared,
		owner:      owner,
		manager:    manager,
		cancel:     cancel,
	}

	switch pipe.EndpointKind {
	case "listener":
		tag := pipe.HandlerTag
		if tag == "" {
			tag = xrayruntime.FrameOutboundTag(pipe.EndpointID)
		}
		if err := manager.RegisterStreamHandler(tag, func(streamCtx context.Context, conn net.Conn) {
			handle.runAcceptedStream(streamCtx, conn, frameKind, guard)
		}); err != nil {
			_ = handle.Close()
			return nil, err
		}
	case "connector":
		if !handle.markActive() {
			_ = handle.Close()
			return nil, fmt.Errorf("core: xray device %s already has an active stream", device.ID)
		}
		conn, err := manager.DialEmbeddedTCP(ctx, pipe.EndpointID, firstNonEmpty(pipe.Remote, "tapx.frame.local"), firstNonZero(pipe.Port, 1))
		if err != nil {
			handle.clearActive()
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
		Type:             device.Type,
		IfName:           tunDevice.Name(),
		MTU:              device.MTU,
		MSSClamp:         device.MSSClamp,
		LinkAutoOptimize: device.LinkAutoOptimize,
		IPv4CIDR:         device.IPv4CIDR,
		IPv6CIDR:         device.IPv6CIDR,
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
		_ = linkdiag.ServeStream(ctx, conn, "")
		return
	}
	defer h.clearActive()
	h.runBridge(ctx, conn, frameKind, guard)
}

func (h *XrayPipeHandle) Diagnose(ctx context.Context, kind string, duration time.Duration) (ConnectorDiagnostic, error) {
	if h.manager == nil {
		return ConnectorDiagnostic{}, errors.New("core: xray diagnostic manager is unavailable")
	}
	conn, err := h.manager.DialEmbeddedTCP(ctx, h.Pipe.EndpointID, firstNonEmpty(h.Pipe.Remote, "tapx.frame.local"), firstNonZero(h.Pipe.Port, 1))
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	defer conn.Close()
	result := ConnectorDiagnostic{Kind: kind, Transport: "xray", Target: net.JoinHostPort(h.Pipe.Remote, fmt.Sprint(h.Pipe.Port))}
	switch kind {
	case "channel":
		result.Delay, err = linkdiag.Ping(ctx, conn, "")
	case "throughput":
		measured, measureErr := linkdiag.Throughput(ctx, conn, "", duration)
		err = measureErr
		result.UploadBytes = measured.UploadBytes
		result.DownloadBytes = measured.DownloadBytes
		result.UploadBPS = measured.UploadBPS
		result.DownloadBPS = measured.DownloadBPS
		result.Duration = measured.Duration
	case "path-mtu":
		result.Delay, err = linkdiag.ProbeFrame(ctx, conn, "", h.Pipe.MaxFrameSize)
		if err == nil {
			result.PathMTU = h.Pipe.DeviceMTU
		}
	default:
		err = fmt.Errorf("core: unsupported xray diagnostic %q", kind)
	}
	return result, err
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
		defer h.clearActive()
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
	conn = applyUserRateLimits(conn, h.Pipe.EndpointKind, h.Pipe.Binding)
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
	maxFrame := maxPositive(h.Pipe.MaxFrameSize, rawtcp.DefaultMaxFrameSize)
	if writer, ok := conn.(xbuf.Writer); ok {
		return h.deviceToXrayWriter(ctx, writer, frameKind, guard, maxFrame)
	}
	frameBuffer := make([]byte, maxFrame)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := readDeviceFrame(ctx, h.device, frameBuffer)
		if err != nil {
			return err
		}
		frame := frameBuffer[:n]
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, true)) {
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

func (h *XrayPipeHandle) deviceToXrayWriter(ctx context.Context, writer xbuf.Writer, frameKind fastpath.FrameKind, guard fastpath.AddressGuard, maxFrame int) error {
	headerSize, err := rawtcp.FrameHeaderSize(h.Pipe.LengthMode)
	if err != nil {
		return err
	}
	bufferSize := maxFrame + headerSize
	const maxBatchFrames = 32
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var buffers [maxBatchFrames]*xbuf.Buffer
		batch := xbuf.MultiBuffer(buffers[:0])
		var batchBytes uint64
		for attempt := 0; attempt < maxBatchFrames; attempt++ {
			pooled := newXrayFrameBuffer(bufferSize)
			storage := pooled.Extend(int32(bufferSize))
			var n int
			if attempt == 0 {
				n, err = readDeviceFrame(ctx, h.device, storage[headerSize:])
			} else {
				n, err = h.device.Read(storage[headerSize:])
			}
			if err == unix.EINTR {
				pooled.Release()
				attempt--
				continue
			}
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				pooled.Release()
				break
			}
			if err != nil {
				pooled.Release()
				xbuf.ReleaseMulti(batch)
				return err
			}
			frame := storage[headerSize : headerSize+n]
			if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, true)) {
				pooled.Release()
				h.counter.dropsGuard.Add(1)
				continue
			}
			if _, err := rawtcp.EncodeFrameHeader(storage[:headerSize], h.Pipe.LengthMode, n); err != nil {
				pooled.Release()
				xbuf.ReleaseMulti(batch)
				return err
			}
			pooled.Resize(0, int32(headerSize+n))
			batch = append(batch, pooled)
			batchBytes += uint64(n)
		}
		if len(batch) == 0 {
			continue
		}
		batchPackets := uint64(len(batch))
		if err := writer.WriteMultiBuffer(batch); err != nil {
			h.counter.dropsIO.Add(1)
			return err
		}
		h.counter.txPackets.Add(batchPackets)
		h.counter.txBytes.Add(batchBytes)
	}
}

func newXrayFrameBuffer(size int) *xbuf.Buffer {
	if size <= xbuf.Size {
		return xbuf.New()
	}
	return xbuf.NewWithSize(int32(size))
}

func (h *XrayPipeHandle) connToDevice(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	maxFrame := maxPositive(h.Pipe.MaxFrameSize, rawtcp.DefaultMaxFrameSize)
	if reader, ok := conn.(xbuf.Reader); ok {
		return h.xrayReaderToDevice(ctx, reader, frameKind, guard, maxFrame)
	}
	frameBuffer := make([]byte, maxFrame)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := rawtcp.ReadFrameInto(conn, h.Pipe.LengthMode, frameBuffer, maxFrame)
		if err != nil {
			return err
		}
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, false)) {
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

func (h *XrayPipeHandle) xrayReaderToDevice(ctx context.Context, reader xbuf.Reader, frameKind fastpath.FrameKind, guard fastpath.AddressGuard, maxFrame int) error {
	framed := xrayMultiBufferFrameReader{reader: reader}
	defer framed.Close()
	scratch := make([]byte, maxFrame)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, release, err := framed.ReadFrame(h.Pipe.LengthMode, scratch, maxFrame)
		if err != nil {
			return err
		}
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, false)) {
			if release != nil {
				release.Release()
			}
			h.counter.dropsGuard.Add(1)
			continue
		}
		_, writeErr := h.device.Write(frame)
		if release != nil {
			release.Release()
		}
		if writeErr != nil {
			h.counter.dropsIO.Add(1)
			return writeErr
		}
		h.counter.rxPackets.Add(1)
		h.counter.rxBytes.Add(uint64(len(frame)))
	}
}

type xrayMultiBufferFrameReader struct {
	reader     xbuf.Reader
	pending    xbuf.MultiBuffer
	pendingErr error
}

func (r *xrayMultiBufferFrameReader) ReadFrame(mode model.TCPLengthMode, scratch []byte, maxFrame int) ([]byte, *xbuf.Buffer, error) {
	headerSize, err := rawtcp.FrameHeaderSize(mode)
	if err != nil {
		return nil, nil, err
	}
	var header [4]byte
	if err := r.readFull(header[:headerSize]); err != nil {
		return nil, nil, err
	}
	frameSize, err := rawtcp.DecodeFrameHeader(header[:headerSize], mode, maxFrame)
	if err != nil {
		return nil, nil, err
	}
	if frameSize > len(scratch) {
		return nil, nil, fmt.Errorf("core: xray frame length %d exceeds scratch buffer %d", frameSize, len(scratch))
	}
	if frameSize == 0 {
		return scratch[:0], nil, nil
	}
	if err := r.fill(); err != nil {
		return nil, nil, err
	}
	buffer := r.pending[0]
	if int(buffer.Len()) >= frameSize {
		frame := buffer.BytesTo(int32(frameSize))
		buffer.Advance(int32(frameSize))
		if buffer.IsEmpty() {
			r.pending[0] = nil
			r.pending = r.pending[1:]
			return frame, buffer, nil
		}
		return frame, nil, nil
	}
	if err := r.readFull(scratch[:frameSize]); err != nil {
		return nil, nil, err
	}
	return scratch[:frameSize], nil, nil
}

func (r *xrayMultiBufferFrameReader) readFull(dst []byte) error {
	for len(dst) > 0 {
		if err := r.fill(); err != nil {
			return err
		}
		buffer := r.pending[0]
		n := copy(dst, buffer.Bytes())
		buffer.Advance(int32(n))
		dst = dst[n:]
		if buffer.IsEmpty() {
			buffer.Release()
			r.pending[0] = nil
			r.pending = r.pending[1:]
		}
	}
	return nil
}

func (r *xrayMultiBufferFrameReader) fill() error {
	r.discardLeadingEmpty()
	for len(r.pending) == 0 {
		if r.pendingErr != nil {
			err := r.pendingErr
			r.pendingErr = nil
			return err
		}
		mb, err := r.reader.ReadMultiBuffer()
		for len(mb) > 0 && (mb[0] == nil || mb[0].IsEmpty()) {
			if mb[0] != nil {
				mb[0].Release()
			}
			mb = mb[1:]
		}
		if len(mb) > 0 {
			r.pending = mb
			r.pendingErr = err
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *xrayMultiBufferFrameReader) discardLeadingEmpty() {
	for len(r.pending) > 0 && (r.pending[0] == nil || r.pending[0].IsEmpty()) {
		if r.pending[0] != nil {
			r.pending[0].Release()
		}
		r.pending[0] = nil
		r.pending = r.pending[1:]
	}
}

func (r *xrayMultiBufferFrameReader) Close() {
	xbuf.ReleaseMulti(r.pending)
	r.pending = nil
}

// Preserve Xray's MultiBuffer fast path when a configured user limit wraps the
// stream. With no configured limit applyUserRateLimits returns the original
// connection and this method is not involved.
func (c *rateLimitedConn) WriteMultiBuffer(mb xbuf.MultiBuffer) error {
	if c.writePacer != nil {
		if err := c.writePacer.wait(int(mb.Len()), c.closed); err != nil {
			xbuf.ReleaseMulti(mb)
			return err
		}
	}
	if writer, ok := c.Conn.(xbuf.Writer); ok {
		return writer.WriteMultiBuffer(mb)
	}
	remaining, err := xbuf.WriteMultiBuffer(c.Conn, mb)
	xbuf.ReleaseMulti(remaining)
	return err
}

func readDeviceFrame(ctx context.Context, device tuntap.Device, buf []byte) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		readN, err := device.Read(buf)
		if err == nil {
			return readN, nil
		}
		if err == unix.EINTR {
			continue
		}
		if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
			return 0, err
		}

		pollFDs := [1]unix.PollFd{{
			Fd:     int32(device.FD()),
			Events: unix.POLLIN,
		}}
		n, err := unix.Poll(pollFDs[:], int((100 * time.Millisecond).Milliseconds()))
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
	if h.owner && h.shared != nil {
		if h.shared.netApply != nil {
			if err := h.shared.netApply.Rollback(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.shared.netApply = nil
		}
		if h.shared.device != nil {
			if err := h.shared.device.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.shared.device = nil
		}
	}
	if h.owner && h.shared == nil && h.device != nil {
		if h.netApply != nil {
			if err := h.netApply.Rollback(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.netApply = nil
		}
		if err := h.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	h.device = nil
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
	if h.shared != nil {
		h.shared.mu.Lock()
		defer h.shared.mu.Unlock()
		if h.shared.active {
			return false
		}
		h.shared.active = true
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active {
		return false
	}
	h.active = true
	return true
}

func (h *XrayPipeHandle) clearActive() {
	if h.shared != nil {
		h.shared.mu.Lock()
		h.shared.active = false
		h.shared.mu.Unlock()
		return
	}
	h.mu.Lock()
	h.active = false
	h.mu.Unlock()
}

func xrayFrameAllowed(kind fastpath.FrameKind, frame []byte, guard fastpath.AddressGuard, sourceAddress bool) bool {
	if len(guard.IPv4Prefixes) == 0 && len(guard.IPv6Prefixes) == 0 && len(guard.MACs) == 0 {
		return true
	}
	switch kind {
	case fastpath.FrameTUN:
		return tunFrameAllowed(frame, guard, sourceAddress)
	case fastpath.FrameTAP:
		return tapFrameAllowed(frame, guard, sourceAddress)
	default:
		return false
	}
}

func tunFrameAllowed(frame []byte, guard fastpath.AddressGuard, sourceAddress bool) bool {
	if len(frame) < 1 {
		return false
	}
	switch frame[0] >> 4 {
	case 4:
		if len(guard.IPv4Prefixes) == 0 {
			return false
		}
		if len(frame) < 20 {
			return false
		}
		offset := 16
		if sourceAddress {
			offset = 12
		}
		return prefixContains4(guard.IPv4Prefixes, netip.AddrFrom4([4]byte{frame[offset], frame[offset+1], frame[offset+2], frame[offset+3]}))
	case 6:
		if len(guard.IPv6Prefixes) == 0 {
			return false
		}
		if len(frame) < 40 {
			return false
		}
		var src [16]byte
		offset := 24
		if sourceAddress {
			offset = 8
		}
		copy(src[:], frame[offset:offset+16])
		return prefixContains6(guard.IPv6Prefixes, netip.AddrFrom16(src))
	default:
		return false
	}
}

func tapFrameAllowed(frame []byte, guard fastpath.AddressGuard, sourceAddress bool) bool {
	if len(frame) < 14 {
		return false
	}
	if len(guard.MACs) > 0 {
		var mac [6]byte
		offset := 0
		if sourceAddress {
			offset = 6
		}
		copy(mac[:], frame[offset:offset+6])
		if !sourceAddress && mac[0]&1 != 0 {
			mac = [6]byte{}
		}
		if mac != ([6]byte{}) && !macAllowed(guard.MACs, mac) {
			return false
		}
	}
	etherType, payload, ok := tapPayload(frame)
	if !ok {
		return false
	}
	switch etherType {
	case 0x0800:
		return tapIPv4Allowed(payload, guard, sourceAddress)
	case 0x0806:
		if len(guard.IPv4Prefixes) == 0 {
			return len(guard.IPv6Prefixes) == 0
		}
		if len(payload) < 28 {
			return false
		}
		offset := 24
		if sourceAddress {
			offset = 14
		}
		return prefixContains4(guard.IPv4Prefixes, netip.AddrFrom4([4]byte{payload[offset], payload[offset+1], payload[offset+2], payload[offset+3]}))
	case 0x86dd:
		return tapIPv6Allowed(payload, guard, sourceAddress)
	case 0x8864:
		protocol, pppPayload, ok := pppoeSessionPayload(payload)
		if !ok {
			return false
		}
		switch protocol {
		case 0x21:
			return tapIPv4Allowed(pppPayload, guard, sourceAddress)
		case 0x57:
			return tapIPv6Allowed(pppPayload, guard, sourceAddress)
		default:
			return true
		}
	default:
		return true
	}
}

func tapPayload(frame []byte) (uint16, []byte, bool) {
	if len(frame) < 14 {
		return 0, nil, false
	}
	etherType := uint16(frame[12])<<8 | uint16(frame[13])
	offset := 14
	for tags := 0; tags < 2 && (etherType == 0x8100 || etherType == 0x88a8 || etherType == 0x9100); tags++ {
		if len(frame) < offset+4 {
			return 0, nil, false
		}
		etherType = uint16(frame[offset+2])<<8 | uint16(frame[offset+3])
		offset += 4
	}
	return etherType, frame[offset:], true
}

func pppoeSessionPayload(payload []byte) (uint16, []byte, bool) {
	if len(payload) < 7 {
		return 0, nil, false
	}
	declared := int(payload[4])<<8 | int(payload[5])
	if declared == 0 || declared > len(payload)-6 {
		return 0, nil, false
	}
	protocolLength := 2
	protocol := uint16(0)
	if payload[6]&1 != 0 {
		protocolLength = 1
		protocol = uint16(payload[6])
	} else {
		if declared < 2 || len(payload) < 8 {
			return 0, nil, false
		}
		protocol = uint16(payload[6])<<8 | uint16(payload[7])
	}
	if declared < protocolLength {
		return 0, nil, false
	}
	return protocol, payload[6+protocolLength : 6+declared], true
}

func tapIPv4Allowed(packet []byte, guard fastpath.AddressGuard, sourceAddress bool) bool {
	if len(guard.IPv4Prefixes) == 0 {
		return len(guard.IPv6Prefixes) == 0
	}
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return false
	}
	offset := 16
	if sourceAddress {
		offset = 12
	}
	return prefixContains4(guard.IPv4Prefixes, netip.AddrFrom4([4]byte{packet[offset], packet[offset+1], packet[offset+2], packet[offset+3]}))
}

func tapIPv6Allowed(packet []byte, guard fastpath.AddressGuard, sourceAddress bool) bool {
	if len(guard.IPv6Prefixes) == 0 {
		return len(guard.IPv4Prefixes) == 0
	}
	if len(packet) < 40 || packet[0]>>4 != 6 {
		return false
	}
	offset := 24
	if sourceAddress {
		offset = 8
	}
	var address [16]byte
	copy(address[:], packet[offset:offset+16])
	return prefixContains6(guard.IPv6Prefixes, netip.AddrFrom16(address))
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
