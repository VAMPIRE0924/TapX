//go:build linux

package core

import (
	"context"
	"errors"
	"fmt"
	"net"

	"tapx/internal/fastpath"
	"tapx/internal/linkdiag"
)

func (h *TCPPipeHandle) startGoListener(frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	listener, local, err := listenTCP(h.Pipe)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.tlsCancel = cancel
	h.listener = listener
	h.LocalAddr = local
	done := make(chan struct{})
	h.acceptDone = done
	go h.acceptGoLoop(ctx, listener, done, frameKind, guard)
	return nil
}

func (h *TCPPipeHandle) acceptGoLoop(ctx context.Context, listener *net.TCPListener, done chan struct{}, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	defer close(done)
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				h.setErr(fmt.Errorf("core: accept go tcp %s: %w", h.Pipe.EndpointID, err))
			}
			return
		}
		if err := configureTCPConn(conn, h.Pipe); err != nil {
			_ = conn.Close()
			h.setErr(err)
			continue
		}
		h.mu.Lock()
		active := h.tlsDone != nil
		h.mu.Unlock()
		if active {
			go func() { _ = linkdiag.ServeStream(ctx, conn, "") }()
			continue
		}
		remote, err := tcpAddrPort(conn.RemoteAddr())
		if err != nil {
			_ = conn.Close()
			h.setErr(err)
			continue
		}
		h.mu.Lock()
		h.acceptedRemote = remote
		h.mu.Unlock()
		if _, err := h.startTLSBridge(ctx, conn, frameKind, guard); err != nil {
			h.setErr(err)
		}
	}
}

func (h *TCPPipeHandle) startGoConnector(frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	conn, local, remote, err := dialTCP(h.Pipe)
	if err != nil {
		return err
	}
	if err := configureTCPConn(conn, h.Pipe); err != nil {
		_ = conn.Close()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.tlsCancel = cancel
	h.LocalAddr = local
	h.RemoteAddr = remote
	if _, err := h.startTLSBridge(ctx, conn, frameKind, guard); err != nil {
		cancel()
		return err
	}
	return nil
}
