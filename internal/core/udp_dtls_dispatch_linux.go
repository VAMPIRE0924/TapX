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

	"github.com/pion/dtls/v3"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/linkdiag"
	"tapx/internal/pathmtu"
)

type dtlsDispatchPolicy struct {
	handle    *UDPPipeHandle
	frameKind fastpath.FrameKind
	guard     fastpath.AddressGuard
}

type DTLSDispatchHandle struct {
	Dispatch config.RuntimeUDPDispatch
	Local    netip.AddrPort

	listener net.Listener
	cancel   context.CancelFunc
	done     chan struct{}
	policies map[string]*dtlsDispatchPolicy
	routes   map[string]string
	fallback string

	mu      sync.Mutex
	pending map[net.Conn]struct{}
	wg      sync.WaitGroup
	lastErr error
}

func startDTLSDispatch(dispatch config.RuntimeUDPDispatch, pipes []config.RuntimeUDPPipe, devices []config.RuntimeDevice, pathCache *pathmtu.Cache) (*DTLSDispatchHandle, []*UDPPipeHandle, error) {
	var prototype *config.RuntimeUDPPipe
	for index := range pipes {
		if pipes[index].DispatchGroup == dispatch.ID {
			prototype = &pipes[index]
			break
		}
	}
	if prototype == nil || !prototype.DTLS.Enabled {
		return nil, nil, fmt.Errorf("core: DTLS dispatch %s has no DTLS worker pipe", dispatch.ID)
	}
	dtlsOptions, err := rawUDPServerDTLSOptions(prototype.DTLS)
	if err != nil {
		return nil, nil, err
	}
	listenAddr, network, err := dtlsDispatchListenAddr(*prototype)
	if err != nil {
		return nil, nil, err
	}
	listener, err := dtls.ListenWithOptions(network, listenAddr, dtlsOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("core: listen DTLS dispatch %s: %w", dispatch.ID, err)
	}
	local, err := addrPortFromNetAddr(listener.Addr())
	if err != nil {
		_ = listener.Close()
		return nil, nil, err
	}

	handle := &DTLSDispatchHandle{
		Dispatch: dispatch, Local: local, listener: listener,
		policies: make(map[string]*dtlsDispatchPolicy), routes: make(map[string]string),
		pending: make(map[net.Conn]struct{}), done: make(chan struct{}),
	}
	for _, route := range dispatch.Routes {
		handle.routes[route.VKeyValue] = fmt.Sprintf("%d", route.SocketIndex)
	}
	if dispatch.FallbackSocketIndex != 0 {
		handle.fallback = fmt.Sprintf("%d", dispatch.FallbackSocketIndex)
	}

	children := make([]*UDPPipeHandle, 0, len(pipes))
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
			_ = listener.Close()
			closeChildren()
			return nil, nil, fmt.Errorf("core: DTLS dispatch %s references missing device %s", dispatch.ID, pipe.DeviceID)
		}
		child, err := prepareUDPPipeHandle(pipe, device, pathCache)
		if err != nil {
			_ = listener.Close()
			closeChildren()
			return nil, nil, err
		}
		frameKind, err := fastpath.FrameKindFromDevice(device.Type)
		if err != nil {
			_ = child.Close()
			_ = listener.Close()
			closeChildren()
			return nil, nil, err
		}
		guard, err := fastpathAddressGuard(pipe.AddressGuard)
		if err != nil {
			_ = child.Close()
			_ = listener.Close()
			closeChildren()
			return nil, nil, err
		}
		policyID := fmt.Sprintf("%d", pipe.DispatchSocketIndex)
		child.LocalAddr = local
		handle.policies[policyID] = &dtlsDispatchPolicy{handle: child, frameKind: frameKind, guard: guard}
		children = append(children, child)
	}
	for _, policyID := range handle.routes {
		if policyID != "0" {
			if _, ok := handle.policies[policyID]; !ok {
				_ = listener.Close()
				closeChildren()
				return nil, nil, fmt.Errorf("core: DTLS dispatch %s references missing policy %s", dispatch.ID, policyID)
			}
		}
	}
	if handle.fallback != "" {
		if _, ok := handle.policies[handle.fallback]; !ok {
			_ = listener.Close()
			closeChildren()
			return nil, nil, fmt.Errorf("core: DTLS dispatch %s fallback references missing policy %s", dispatch.ID, handle.fallback)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	handle.cancel = cancel
	go handle.acceptLoop(ctx)
	return handle, children, nil
}

func dtlsDispatchListenAddr(pipe config.RuntimeUDPPipe) (*net.UDPAddr, string, error) {
	host := firstNonEmpty(pipe.BindAddress, pipe.BindHost, "0.0.0.0")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil, "", fmt.Errorf("core: parse DTLS dispatch bind address %q: %w", host, err)
	}
	network := "udp4"
	if addr.Is6() {
		network = "udp6"
	}
	return net.UDPAddrFromAddrPort(netip.AddrPortFrom(addr, pipe.BindPort)), network, nil
}

func (h *DTLSDispatchHandle) acceptLoop(ctx context.Context) {
	defer close(h.done)
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
				h.setErr(fmt.Errorf("core: accept DTLS dispatch %s: %w", h.Dispatch.ID, err))
			}
			return
		}
		h.mu.Lock()
		h.pending[conn] = struct{}{}
		h.mu.Unlock()
		h.wg.Add(1)
		go h.dispatchConn(ctx, conn)
	}
}

func (h *DTLSDispatchHandle) dispatchConn(ctx context.Context, conn net.Conn) {
	defer h.wg.Done()
	defer h.removePending(conn)
	dtlsConn, ok := conn.(*dtls.Conn)
	if !ok {
		_ = conn.Close()
		h.setErr(fmt.Errorf("core: DTLS dispatch accepted %T", conn))
		return
	}
	prototype := h.prototypePolicy()
	if prototype == nil {
		_ = conn.Close()
		return
	}
	handshakeCtx, cancel := prototype.handle.dtlsHandshakeContext(ctx)
	err := dtlsConn.HandshakeContext(handshakeCtx)
	cancel()
	if err != nil {
		_ = conn.Close()
		if ctx.Err() == nil {
			h.setErr(fmt.Errorf("core: DTLS dispatch handshake %s: %w", h.Dispatch.ID, err))
		}
		return
	}
	hello, err := receiveDTLSVKeyHello(ctx, conn, prototype.handle.dtlsHandshakeTimeout())
	if err != nil {
		_ = conn.Close()
		if ctx.Err() == nil {
			h.setErr(fmt.Errorf("core: classify DTLS dispatch %s: %w", h.Dispatch.ID, err))
		}
		return
	}
	policy := h.policyForVKey(hello.vkey)
	if policy == nil {
		_ = conn.Close()
		return
	}
	if hello.diagnostic {
		if err := linkdiag.ServeStream(ctx, conn, hello.vkey); err != nil && ctx.Err() == nil &&
			!errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			h.setErr(fmt.Errorf("core: serve DTLS diagnostic %s: %w", h.Dispatch.ID, err))
		}
		_ = conn.Close()
		return
	}
	remote, err := addrPortFromNetAddr(conn.RemoteAddr())
	if err != nil {
		_ = conn.Close()
		h.setErr(err)
		return
	}
	if err := policy.handle.confirmDTLSPath(ctx, dtlsConn, remote); err != nil {
		_ = conn.Close()
		policy.handle.setErr(err)
		return
	}
	if err := policy.handle.startAcceptedDTLSBridge(ctx, conn, remote, policy.frameKind, policy.guard); err != nil {
		policy.handle.setErr(err)
		h.setErr(err)
	}
}

func (h *DTLSDispatchHandle) prototypePolicy() *dtlsDispatchPolicy {
	for _, policy := range h.policies {
		return policy
	}
	return nil
}

func (h *DTLSDispatchHandle) policyForVKey(vkey string) *dtlsDispatchPolicy {
	if vkey == "" {
		return h.policies[h.fallback]
	}
	policyID, ok := h.routes[vkey]
	if !ok || policyID == "0" {
		return nil
	}
	return h.policies[policyID]
}

func (h *DTLSDispatchHandle) removePending(conn net.Conn) {
	h.mu.Lock()
	delete(h.pending, conn)
	h.mu.Unlock()
}

func (h *DTLSDispatchHandle) setErr(err error) {
	h.mu.Lock()
	if h.lastErr == nil {
		h.lastErr = err
	}
	h.mu.Unlock()
}

func (h *DTLSDispatchHandle) Close() error {
	if h == nil {
		return nil
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	var firstErr error
	if h.listener != nil {
		if err := h.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
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

func (h *UDPPipeHandle) startAcceptedDTLSBridge(ctx context.Context, conn net.Conn, remote netip.AddrPort, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	h.mu.Lock()
	if h.dtlsDone != nil {
		h.mu.Unlock()
		_ = conn.Close()
		return fmt.Errorf("core: DTLS route %s already has an active session", h.Pipe.RouteID)
	}
	done := make(chan struct{})
	h.dtlsDone = done
	h.dtlsConn = conn
	h.RemoteAddr = remote
	h.acceptedRemote = remote
	h.mu.Unlock()
	go func() {
		defer close(done)
		h.runDTLSBridge(ctx, conn, frameKind, guard)
		h.mu.Lock()
		if h.dtlsConn == conn {
			h.dtlsConn = nil
		}
		if h.dtlsDone == done {
			h.dtlsDone = nil
		}
		h.mu.Unlock()
	}()
	return nil
}
