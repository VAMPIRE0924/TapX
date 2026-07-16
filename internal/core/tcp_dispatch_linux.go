//go:build linux

package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/linkdiag"
	"tapx/internal/model"
	"tapx/internal/rawtcp"
)

const rawVKeyMaxSize = 1024

type tcpDispatchProbe struct {
	Diagnostic bool
	VKey       string
}

type tcpDispatchPolicy struct {
	handle     *TCPPipeHandle
	frameKind  fastpath.FrameKind
	lengthMode fastpath.TCPLengthMode
	guard      fastpath.AddressGuard
}

type TCPDispatchHandle struct {
	Dispatch  config.RuntimeTCPDispatch
	LocalAddr netip.AddrPort

	listener *net.TCPListener
	policies map[string]*tcpDispatchPolicy
	routes   map[string]string
	fallback string
	done     chan struct{}

	mu        sync.Mutex
	pending   map[*net.TCPConn]struct{}
	wg        sync.WaitGroup
	lastErr   error
	tlsConfig *tls.Config
}

type bufferedDispatchConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedDispatchConn) Read(payload []byte) (int, error) {
	return c.reader.Read(payload)
}

func startTCPDispatch(dispatch config.RuntimeTCPDispatch, pipes []config.RuntimeTCPPipe, devices []config.RuntimeDevice) (*TCPDispatchHandle, []*TCPPipeHandle, error) {
	if dispatch.ID == "" || dispatch.EndpointID == "" {
		return nil, nil, errors.New("core: TCP dispatch ID and endpoint are required")
	}
	var prototype *config.RuntimeTCPPipe
	for index := range pipes {
		if pipes[index].DispatchGroup == dispatch.ID {
			prototype = &pipes[index]
			break
		}
	}
	if prototype == nil {
		return nil, nil, fmt.Errorf("core: TCP dispatch %s has no worker pipe", dispatch.ID)
	}
	if prototype.ExternalXrayBridge {
		return nil, nil, fmt.Errorf("core: TCP dispatch %s cannot use an external Xray bridge", dispatch.ID)
	}

	handle := &TCPDispatchHandle{
		Dispatch: dispatch, policies: make(map[string]*tcpDispatchPolicy), routes: make(map[string]string),
		fallback: dispatch.FallbackPolicyID, done: make(chan struct{}), pending: make(map[*net.TCPConn]struct{}),
	}
	if prototype.TLS.Enabled {
		tlsConfig, err := rawTCPServerTLSConfig(prototype.TLS)
		if err != nil {
			return nil, nil, err
		}
		handle.tlsConfig = tlsConfig
	}
	for _, route := range dispatch.Routes {
		handle.routes[route.VKeyValue] = route.PolicyID
	}

	children := make([]*TCPPipeHandle, 0, len(pipes))
	sharedDevices := make(map[string]*tcpSharedDevice)
	closeChildren := func() {
		for index := len(children) - 1; index >= 0; index-- {
			_ = children[index].Close()
		}
	}
	for _, pipe := range pipes {
		if pipe.DispatchGroup != dispatch.ID {
			continue
		}
		device, ok := findRuntimeDevice(devices, pipe.DeviceID)
		if !ok {
			closeChildren()
			return nil, nil, fmt.Errorf("core: TCP dispatch %s policy %s references missing device %s", dispatch.ID, pipe.DispatchPolicyID, pipe.DeviceID)
		}
		child, err := prepareTCPPipeHandle(pipe, device, sharedDevices[pipe.DeviceID])
		if err != nil {
			closeChildren()
			return nil, nil, err
		}
		if child.owner {
			sharedDevices[pipe.DeviceID] = child.shared
		}
		frameKind, err := fastpath.FrameKindFromDevice(device.Type)
		if err != nil {
			_ = child.Close()
			closeChildren()
			return nil, nil, err
		}
		lengthMode, err := fastpath.TCPLengthModeFromModel(pipe.LengthMode)
		if err != nil {
			_ = child.Close()
			closeChildren()
			return nil, nil, err
		}
		guard, err := fastpathAddressGuard(pipe.AddressGuard)
		if err != nil {
			_ = child.Close()
			closeChildren()
			return nil, nil, err
		}
		if pipe.DispatchPolicyID == "" {
			_ = child.Close()
			closeChildren()
			return nil, nil, fmt.Errorf("core: TCP dispatch %s contains an empty policy ID", dispatch.ID)
		}
		if _, exists := handle.policies[pipe.DispatchPolicyID]; exists {
			_ = child.Close()
			closeChildren()
			return nil, nil, fmt.Errorf("core: TCP dispatch %s duplicates policy %s", dispatch.ID, pipe.DispatchPolicyID)
		}
		handle.policies[pipe.DispatchPolicyID] = &tcpDispatchPolicy{
			handle: child, frameKind: frameKind, lengthMode: lengthMode, guard: guard,
		}
		children = append(children, child)
	}
	for key, policyID := range handle.routes {
		if policyID != "" {
			if _, ok := handle.policies[policyID]; !ok {
				closeChildren()
				return nil, nil, fmt.Errorf("core: TCP dispatch %s vKey %q references missing policy %s", dispatch.ID, key, policyID)
			}
		}
	}
	if handle.fallback != "" {
		if _, ok := handle.policies[handle.fallback]; !ok {
			closeChildren()
			return nil, nil, fmt.Errorf("core: TCP dispatch %s fallback references missing policy %s", dispatch.ID, handle.fallback)
		}
	}

	listener, local, err := listenTCP(*prototype)
	if err != nil {
		closeChildren()
		return nil, nil, err
	}
	handle.listener = listener
	handle.LocalAddr = local
	for _, child := range children {
		child.LocalAddr = local
	}
	go handle.acceptLoop()
	return handle, children, nil
}

func (h *TCPDispatchHandle) acceptLoop() {
	defer close(h.done)
	for {
		conn, err := h.listener.AcceptTCP()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				h.setErr(fmt.Errorf("core: accept TCP dispatch %s: %w", h.Dispatch.ID, err))
			}
			return
		}
		h.mu.Lock()
		h.pending[conn] = struct{}{}
		h.mu.Unlock()
		h.wg.Add(1)
		go h.dispatchConn(conn)
	}
}

func (h *TCPDispatchHandle) dispatchConn(conn *net.TCPConn) {
	defer h.wg.Done()
	defer h.removePending(conn)
	prototype := h.prototypePolicy()
	if prototype == nil {
		_ = conn.Close()
		return
	}
	timeout := time.Duration(prototype.handle.Pipe.ConnectTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	var stream net.Conn = conn
	var probe tcpDispatchProbe
	var err error
	if h.tlsConfig != nil {
		if err = configureTCPConn(conn, prototype.handle.Pipe); err == nil {
			tlsConn := tls.Server(conn, h.tlsConfig)
			handshakeCtx, cancel := context.WithTimeout(context.Background(), timeout)
			err = tlsConn.HandshakeContext(handshakeCtx)
			cancel()
			if err == nil {
				reader := bufio.NewReaderSize(tlsConn, 4+rawVKeyHeaderBaseSize+rawVKeyMaxSize)
				probe, err = peekTCPDispatchReader(reader, prototype.handle.Pipe.LengthMode, prototype.handle.Pipe.MaxFrameSize)
				stream = &bufferedDispatchConn{Conn: tlsConn, reader: reader}
			}
		}
	} else {
		probe, err = peekTCPDispatch(conn, prototype.handle.Pipe.LengthMode, prototype.handle.Pipe.MaxFrameSize, timeout)
	}
	if err != nil {
		_ = conn.Close()
		if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			h.setErr(fmt.Errorf("core: classify TCP dispatch %s: %w", h.Dispatch.ID, err))
		}
		return
	}
	_ = stream.SetReadDeadline(time.Time{})
	policy := h.policyForVKey(probe.VKey)
	if policy == nil {
		_ = conn.Close()
		return
	}
	if probe.Diagnostic {
		_ = linkdiag.ServeStream(context.Background(), stream, probe.VKey)
		return
	}
	remote, err := tcpAddrPort(conn.RemoteAddr())
	if err != nil {
		_ = conn.Close()
		h.setErr(err)
		return
	}
	policy.handle.mu.Lock()
	policy.handle.acceptedRemote = remote
	policy.handle.mu.Unlock()
	if h.tlsConfig != nil {
		if _, err := policy.handle.startTLSBridge(context.Background(), stream, policy.frameKind, policy.guard); err != nil {
			h.setErr(err)
		}
		return
	}
	if err := policy.handle.startWorkerFromConn(conn, policy.frameKind, policy.lengthMode, policy.guard); err != nil {
		h.setErr(err)
	}
}

func (h *TCPDispatchHandle) prototypePolicy() *tcpDispatchPolicy {
	if h.fallback != "" {
		if policy := h.policies[h.fallback]; policy != nil {
			return policy
		}
	}
	for _, policy := range h.policies {
		return policy
	}
	return nil
}

func (h *TCPDispatchHandle) policyForVKey(vkey string) *tcpDispatchPolicy {
	if vkey == "" {
		return h.policies[h.fallback]
	}
	policyID, ok := h.routes[vkey]
	if !ok || policyID == "" {
		return nil
	}
	return h.policies[policyID]
}

func (h *TCPDispatchHandle) removePending(conn *net.TCPConn) {
	h.mu.Lock()
	delete(h.pending, conn)
	h.mu.Unlock()
}

func (h *TCPDispatchHandle) Close() error {
	var firstErr error
	if h.listener != nil {
		if err := h.listener.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.listener = nil
	}
	h.mu.Lock()
	for conn := range h.pending {
		_ = conn.Close()
	}
	h.mu.Unlock()
	if h.done != nil {
		<-h.done
		h.done = nil
	}
	h.wg.Wait()
	return firstErr
}

func (h *TCPDispatchHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErr
}

func (h *TCPDispatchHandle) setErr(err error) {
	h.mu.Lock()
	if h.lastErr == nil {
		h.lastErr = err
	}
	h.mu.Unlock()
}

func peekTCPDispatch(conn *net.TCPConn, mode model.TCPLengthMode, maxFrame int, timeout time.Duration) (tcpDispatchProbe, error) {
	deadline := time.Now().Add(timeout)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return tcpDispatchProbe{}, err
	}
	buffer := make([]byte, 4+rawVKeyHeaderBaseSize+rawVKeyMaxSize)
	for {
		n, err := peekTCPBytes(conn, buffer)
		if err != nil {
			return tcpDispatchProbe{}, err
		}
		if n == 0 {
			return tcpDispatchProbe{}, io.EOF
		}
		probe, complete, required, err := inspectTCPDispatchPrefix(buffer[:n], mode, maxFrame)
		if err != nil {
			return tcpDispatchProbe{}, err
		}
		if complete {
			return probe, nil
		}
		if required > len(buffer) {
			return tcpDispatchProbe{}, fmt.Errorf("core: TCP dispatch prefix requires %d bytes, limit is %d", required, len(buffer))
		}
		if !time.Now().Before(deadline) {
			return tcpDispatchProbe{}, fmt.Errorf("core: TCP dispatch prefix timed out waiting for %d bytes", required)
		}
		time.Sleep(time.Millisecond)
	}
}

func peekTCPDispatchReader(reader *bufio.Reader, mode model.TCPLengthMode, maxFrame int) (tcpDispatchProbe, error) {
	required := 1
	for {
		payload, err := reader.Peek(required)
		if err != nil {
			return tcpDispatchProbe{}, err
		}
		probe, complete, nextRequired, err := inspectTCPDispatchPrefix(payload, mode, maxFrame)
		if err != nil {
			return tcpDispatchProbe{}, err
		}
		if complete {
			return probe, nil
		}
		if nextRequired <= required {
			nextRequired = required + 1
		}
		if nextRequired > reader.Size() {
			return tcpDispatchProbe{}, fmt.Errorf("core: TCP dispatch prefix requires %d bytes, limit is %d", nextRequired, reader.Size())
		}
		required = nextRequired
	}
}

func inspectTCPDispatchPrefix(payload []byte, mode model.TCPLengthMode, maxFrame int) (tcpDispatchProbe, bool, int, error) {
	diagnostic, err := linkdiag.InspectStreamHelloPrefix(payload)
	if err != nil {
		return tcpDispatchProbe{}, false, diagnostic.Required, err
	}
	if diagnostic.Matched {
		return tcpDispatchProbe{Diagnostic: true, VKey: diagnostic.Credential}, diagnostic.Complete, diagnostic.Required, nil
	}
	headerSize, err := rawtcp.FrameHeaderSize(mode)
	if err != nil {
		return tcpDispatchProbe{}, false, 0, err
	}
	if len(payload) < headerSize {
		return tcpDispatchProbe{}, false, headerSize, nil
	}
	if maxFrame <= 0 {
		maxFrame = rawtcp.DefaultMaxFrameSize
	}
	maxWireFrame := maxFrame + rawVKeyHeaderBaseSize + rawVKeyMaxSize
	frameSize, err := rawtcp.DecodeFrameHeader(payload[:headerSize], mode, maxWireFrame)
	if err != nil {
		return tcpDispatchProbe{}, false, headerSize, err
	}
	if frameSize < 4 {
		if frameSize > maxFrame {
			return tcpDispatchProbe{}, false, headerSize, fmt.Errorf("core: raw TCP frame %d exceeds max %d", frameSize, maxFrame)
		}
		return tcpDispatchProbe{}, true, headerSize, nil
	}
	required := headerSize + 4
	if len(payload) < required {
		return tcpDispatchProbe{}, false, required, nil
	}
	frame := payload[headerSize:]
	if string(frame[:4]) != rawVKeyMagic {
		if frameSize > maxFrame {
			return tcpDispatchProbe{}, false, required, fmt.Errorf("core: raw TCP frame %d exceeds max %d", frameSize, maxFrame)
		}
		return tcpDispatchProbe{}, true, required, nil
	}
	if frameSize < rawVKeyHeaderBaseSize {
		return tcpDispatchProbe{}, false, required, errors.New("core: truncated raw TCP vKey header")
	}
	required = headerSize + rawVKeyHeaderBaseSize
	if len(payload) < required {
		return tcpDispatchProbe{}, false, required, nil
	}
	keySize := int(binary.BigEndian.Uint16(frame[4:6]))
	if keySize == 0 || keySize > rawVKeyMaxSize || frame[6] != 0 || frame[7] != 0 {
		return tcpDispatchProbe{}, false, required, errors.New("core: invalid raw TCP vKey header")
	}
	if frameSize < rawVKeyHeaderBaseSize+keySize || frameSize-rawVKeyHeaderBaseSize-keySize > maxFrame {
		return tcpDispatchProbe{}, false, required, fmt.Errorf("core: invalid raw TCP vKey frame length %d", frameSize)
	}
	required += keySize
	if len(payload) < required {
		return tcpDispatchProbe{}, false, required, nil
	}
	return tcpDispatchProbe{VKey: string(frame[rawVKeyHeaderBaseSize : rawVKeyHeaderBaseSize+keySize])}, true, required, nil
}

func peekTCPBytes(conn *net.TCPConn, buffer []byte) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var (
		n       int
		peekErr error
	)
	err = raw.Read(func(fd uintptr) bool {
		n, _, peekErr = unix.Recvfrom(int(fd), buffer, unix.MSG_PEEK)
		return peekErr != unix.EAGAIN && peekErr != unix.EWOULDBLOCK
	})
	if err != nil {
		return 0, err
	}
	return n, peekErr
}
