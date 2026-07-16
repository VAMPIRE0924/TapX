//go:build linux

package core

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/pathmtu"
)

const (
	defaultUDPProbeTimeout = 500 * time.Millisecond
	defaultUDPCommitGrace  = 1600 * time.Millisecond
	defaultUDPHandoffDelay = 1700 * time.Millisecond
)

type rawUDPPathPreparer struct {
	cache        *pathmtu.Cache
	runner       pathmtu.CommandRunner
	probeTimeout time.Duration
	commitGrace  time.Duration
	handoffDelay time.Duration
}

func defaultRawUDPPathPreparer(cache *pathmtu.Cache) rawUDPPathPreparer {
	return rawUDPPathPreparer{
		cache: cache, probeTimeout: defaultUDPProbeTimeout,
		commitGrace: defaultUDPCommitGrace, handoffDelay: defaultUDPHandoffDelay,
	}
}

func (p rawUDPPathPreparer) prepare(ctx context.Context, pipe config.RuntimeUDPPipe, device config.RuntimeDevice, conn *net.UDPConn, peer netip.AddrPort) (config.RuntimeUDPPipe, netip.AddrPort, error) {
	if conn == nil {
		return pipe, netip.AddrPort{}, fmt.Errorf("core: UDP path probe connection is required")
	}
	confirmer := &pathmtu.Confirmer{Cache: p.cache, Runner: p.runner}
	prefix, err := rawUDPPathProbePrefix(pipe.Binding.VKeyValue)
	if err != nil {
		return pipe, netip.AddrPort{}, fmt.Errorf("core: build UDP path probe prefix: %w", err)
	}
	ceiling, err := pathmtu.RawUDPFramePayloadCeiling(device.MTU, device.Type == model.DeviceTAP, len([]byte(pipe.Binding.VKeyValue)))
	if err != nil {
		return pipe, netip.AddrPort{}, fmt.Errorf("core: calculate UDP path probe ceiling: %w", err)
	}
	if peer.IsValid() {
		input := rawUDPConfirmationInput(pipe, device, peer)
		input.ProbePrefixSize = len(prefix)
		exchanger := &pathmtu.UDPExchanger{
			Conn: conn, Peer: net.UDPAddrFromAddrPort(peer), Timeout: positiveDuration(p.probeTimeout, defaultUDPProbeTimeout),
			MaxPayload: ceiling, Prefix: prefix,
		}
		entry, err := confirmer.ConfirmAndCommitRawUDP(ctx, input, exchanger)
		if err != nil {
			return pipe, netip.AddrPort{}, fmt.Errorf("core: confirm UDP %s path %s: %w", pipe.EndpointKind, pipe.EndpointID, err)
		}
		if err := exchanger.Handoff(ctx, positiveDuration(p.handoffDelay, defaultUDPHandoffDelay)); err != nil {
			return pipe, netip.AddrPort{}, err
		}
		return applyConfirmedUDPPath(pipe, entry), peer, nil
	}
	switch pipe.EndpointKind {
	case "connector":
		return pipe, netip.AddrPort{}, fmt.Errorf("core: UDP connector %s requires a peer for path confirmation", pipe.EndpointID)
	case "listener":
		confirmed := make(chan pathmtu.UDPConfirmedPath, 1)
		responderErr := make(chan error, 1)
		responderCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			responderErr <- pathmtu.ServeUDPProbeResponses(responderCtx, conn, pathmtu.UDPResponderOptions{
				MaxPayload: ceiling, Prefix: prefix, CommitGrace: positiveDuration(p.commitGrace, defaultUDPCommitGrace),
				OnConfirmed: func(value pathmtu.UDPConfirmedPath) {
					select {
					case confirmed <- value:
					default:
					}
				},
			})
		}()
		var committed pathmtu.UDPConfirmedPath
		select {
		case committed = <-confirmed:
		case err := <-responderErr:
			return pipe, netip.AddrPort{}, fmt.Errorf("core: serve UDP listener path probe %s: %w", pipe.EndpointID, err)
		case <-ctx.Done():
			return pipe, netip.AddrPort{}, ctx.Err()
		}
		input := rawUDPConfirmationInput(pipe, device, committed.Peer)
		input.ProbePrefixSize = len(prefix)
		entry, err := confirmer.AcceptRawUDPCommit(ctx, input, committed)
		if err != nil {
			cancel()
			<-responderErr
			return pipe, netip.AddrPort{}, fmt.Errorf("core: accept UDP listener path %s: %w", pipe.EndpointID, err)
		}
		select {
		case err := <-responderErr:
			if err != nil {
				return pipe, netip.AddrPort{}, fmt.Errorf("core: finish UDP listener path probe %s: %w", pipe.EndpointID, err)
			}
		case <-ctx.Done():
			return pipe, netip.AddrPort{}, ctx.Err()
		}
		return applyConfirmedUDPPath(pipe, entry), committed.Peer, nil
	default:
		return pipe, netip.AddrPort{}, fmt.Errorf("core: UDP pipe %s has unsupported endpoint kind %q", pipe.EndpointID, pipe.EndpointKind)
	}
}

func rawUDPPathProbePrefix(vkey string) ([]byte, error) {
	value := []byte(vkey)
	size, err := rawVKeyHeaderSize(value)
	if err != nil || size == 0 {
		return nil, err
	}
	prefix := make([]byte, size)
	writeRawVKeyHeader(prefix, value)
	return prefix, nil
}

func (p rawUDPPathPreparer) prepareConn(ctx context.Context, pipe config.RuntimeUDPPipe, device config.RuntimeDevice, conn net.Conn, peer netip.AddrPort, securityOverhead int) (config.RuntimeUDPPipe, error) {
	if conn == nil {
		return pipe, fmt.Errorf("core: DTLS path probe connection is required")
	}
	if !peer.IsValid() {
		return pipe, fmt.Errorf("core: DTLS path probe peer is required")
	}
	confirmer := &pathmtu.Confirmer{Cache: p.cache, Runner: p.runner}
	input := udpConfirmationInput(pipe, device, peer, "raw-udp-dtls", securityOverhead)
	switch pipe.EndpointKind {
	case "connector":
		exchanger := &pathmtu.ConnExchanger{Conn: conn, Timeout: positiveDuration(p.probeTimeout, defaultUDPProbeTimeout)}
		entry, err := confirmer.ConfirmAndCommitRawUDP(ctx, input, exchanger)
		if err != nil {
			return pipe, fmt.Errorf("core: confirm DTLS connector path %s: %w", pipe.EndpointID, err)
		}
		if err := waitForPathHandoff(ctx, positiveDuration(p.handoffDelay, defaultUDPHandoffDelay)); err != nil {
			return pipe, err
		}
		return applyConfirmedUDPPath(pipe, entry), nil
	case "listener":
		ceiling, err := pathmtu.RawUDPFramePayloadCeiling(device.MTU, device.Type == model.DeviceTAP, len([]byte(pipe.Binding.VKeyValue)))
		if err != nil {
			return pipe, fmt.Errorf("core: calculate DTLS listener probe ceiling: %w", err)
		}
		confirmed := make(chan pathmtu.CommittedProbe, 1)
		responderErr := make(chan error, 1)
		responderCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			responderErr <- pathmtu.ServeConnProbeResponses(responderCtx, conn, pathmtu.ConnResponderOptions{
				MaxPayload: ceiling, CommitGrace: positiveDuration(p.commitGrace, defaultUDPCommitGrace),
				OnConfirmed: func(value pathmtu.CommittedProbe) {
					select {
					case confirmed <- value:
					default:
					}
				},
			})
		}()
		var committed pathmtu.CommittedProbe
		select {
		case committed = <-confirmed:
		case err := <-responderErr:
			return pipe, fmt.Errorf("core: serve DTLS listener path probe %s: %w", pipe.EndpointID, err)
		case <-ctx.Done():
			return pipe, ctx.Err()
		}
		entry, err := confirmer.AcceptRawUDPCommit(ctx, input, pathmtu.UDPConfirmedPath{
			Peer: peer, PayloadSize: committed.PayloadSize, Token: committed.Token,
		})
		if err != nil {
			cancel()
			<-responderErr
			return pipe, fmt.Errorf("core: accept DTLS listener path %s: %w", pipe.EndpointID, err)
		}
		select {
		case err := <-responderErr:
			if err != nil {
				return pipe, fmt.Errorf("core: finish DTLS listener path probe %s: %w", pipe.EndpointID, err)
			}
		case <-ctx.Done():
			return pipe, ctx.Err()
		}
		return applyConfirmedUDPPath(pipe, entry), nil
	default:
		return pipe, fmt.Errorf("core: DTLS pipe %s has unsupported endpoint kind %q", pipe.EndpointID, pipe.EndpointKind)
	}
}

func rawUDPConfirmationInput(pipe config.RuntimeUDPPipe, device config.RuntimeDevice, peer netip.AddrPort) pathmtu.RawUDPConfirmationInput {
	return udpConfirmationInput(pipe, device, peer, "raw-udp", 0)
}

func udpConfirmationInput(pipe config.RuntimeUDPPipe, device config.RuntimeDevice, peer netip.AddrPort, transport string, securityOverhead int) pathmtu.RawUDPConfirmationInput {
	minimumMTU := 576
	if peer.Addr().Is6() {
		minimumMTU = 1280
	}
	return pathmtu.RawUDPConfirmationInput{
		Key: pathmtu.PathKey{
			EndpointKind: pipe.EndpointKind, EndpointID: pipe.EndpointID, DeviceID: pipe.DeviceID,
			Transport: transport, Remote: peer,
		},
		DeviceMTUCeiling: device.MTU, MinimumNetworkMTU: minimumMTU,
		TAP: device.Type == model.DeviceTAP, VKeyLength: len([]byte(pipe.Binding.VKeyValue)),
		SecurityOverhead: securityOverhead, Attempts: 3,
	}
}

func applyConfirmedUDPPath(pipe config.RuntimeUDPPipe, entry pathmtu.ConfirmedPath) config.RuntimeUDPPipe {
	pipe.MaxDatagramPayload = entry.Plan.MaxWirePayload
	pipe.ConfirmedPathMTU = entry.Plan.PathMTU
	pipe.EffectiveNetworkMTU = entry.Plan.EffectiveNetworkMTU
	pipe.TCPMSSIPv4 = entry.Plan.TCPMSSIPv4
	pipe.TCPMSSIPv6 = entry.Plan.TCPMSSIPv6
	return pipe
}

func duplicateUDPConn(fd int) (*net.UDPConn, error) {
	duplicate, err := unix.Dup(fd)
	if err != nil {
		return nil, fmt.Errorf("duplicate UDP socket: %w", err)
	}
	unix.CloseOnExec(duplicate)
	file := os.NewFile(uintptr(duplicate), "tapx-pathmtu-udp")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, fmt.Errorf("wrap duplicated UDP socket")
	}
	packetConn, err := net.FilePacketConn(file)
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("create UDP probe connection: %w", err)
	}
	udpConn, ok := packetConn.(*net.UDPConn)
	if !ok {
		_ = packetConn.Close()
		return nil, fmt.Errorf("duplicated socket is %T, want UDP", packetConn)
	}
	return udpConn, nil
}

func waitForPathHandoff(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func positiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
