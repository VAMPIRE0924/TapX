package pathmtu

import (
	"fmt"
	"net/netip"
)

const (
	IPv4HeaderSize     = 20
	IPv6HeaderSize     = 40
	UDPHeaderSize      = 8
	TCPHeaderSize      = 20
	EthernetHeaderSize = 14
	VKeyHeaderBaseSize = 8
	SegmentHeaderSize  = 20
)

// DatagramInput contains only immutable values known after a path has been
// confirmed. SecurityOverhead is supplied by the selected transport because
// DTLS and managed datagram transports do not share one fixed record overhead.
type DatagramInput struct {
	PathMTU          int
	DeviceMTUCeiling int
	OuterAddress     netip.Addr
	TAP              bool
	VKeyLength       int
	SecurityOverhead int
}

type DatagramPlan struct {
	PathMTU             int
	OuterIPHeader       int
	TransportOverhead   int
	ControlOverhead     int
	FrameOverhead       int
	MaxWirePayload      int
	MaxFrameSize        int
	MaxSegmentFrameSize int
	EffectiveNetworkMTU int
	TCPMSSIPv4          int
	TCPMSSIPv6          int
}

type RawUDPProbeInput struct {
	DeviceMTUCeiling  int
	RouteMTUCandidate int
	MinimumNetworkMTU int
	OuterAddress      netip.Addr
	TAP               bool
	VKeyLength        int
	ProbePrefixSize   int
	SecurityOverhead  int
	Attempts          int
}

func RawUDPFramePayloadCeiling(deviceMTU int, tap bool, vkeyLength int) (int, error) {
	if deviceMTU <= 0 {
		return 0, fmt.Errorf("device MTU ceiling must be positive")
	}
	if vkeyLength < 0 {
		return 0, fmt.Errorf("vKey length must not be negative")
	}
	overhead := SegmentHeaderSize
	if tap {
		overhead += EthernetHeaderSize
	}
	if vkeyLength > 0 {
		overhead += VKeyHeaderBaseSize + vkeyLength
	}
	return deviceMTU + overhead, nil
}

// RawUDPConfirmOptions converts operator and route ceilings into UDP payload
// sizes for ConfirmPayload. RouteMTUCandidate is deliberately only the second
// attempt; the device ceiling is always tried first and only peer responses can
// promote any size to a confirmed result.
func RawUDPConfirmOptions(input RawUDPProbeInput) (ConfirmOptions, error) {
	if input.DeviceMTUCeiling <= 0 {
		return ConfirmOptions{}, fmt.Errorf("device MTU ceiling must be positive")
	}
	if input.RouteMTUCandidate <= 0 {
		return ConfirmOptions{}, fmt.Errorf("route MTU candidate must be positive")
	}
	if input.MinimumNetworkMTU <= 0 {
		return ConfirmOptions{}, fmt.Errorf("minimum network MTU must be positive")
	}
	if !input.OuterAddress.IsValid() {
		return ConfirmOptions{}, fmt.Errorf("outer address is required")
	}
	if input.VKeyLength < 0 {
		return ConfirmOptions{}, fmt.Errorf("vKey length must not be negative")
	}
	if input.SecurityOverhead < 0 {
		return ConfirmOptions{}, fmt.Errorf("security overhead must not be negative")
	}
	if input.ProbePrefixSize < 0 {
		return ConfirmOptions{}, fmt.Errorf("probe prefix size must not be negative")
	}

	desiredPayload, err := RawUDPFramePayloadCeiling(input.DeviceMTUCeiling, input.TAP, input.VKeyLength)
	if err != nil {
		return ConfirmOptions{}, err
	}
	desiredPayload -= input.ProbePrefixSize
	if desiredPayload < ProbeHeaderSize {
		return ConfirmOptions{}, fmt.Errorf("probe prefix leaves no usable probe payload")
	}
	ipHeader := IPv4HeaderSize
	if input.OuterAddress.Is6() {
		ipHeader = IPv6HeaderSize
	}
	minimumPayload := input.MinimumNetworkMTU - ipHeader - UDPHeaderSize - input.SecurityOverhead - input.ProbePrefixSize
	if minimumPayload < ProbeHeaderSize {
		minimumPayload = ProbeHeaderSize
	}
	if minimumPayload > desiredPayload {
		minimumPayload = desiredPayload
	}

	return ConfirmOptions{
		DesiredPayload:   desiredPayload,
		CandidatePayload: input.RouteMTUCandidate - ipHeader - UDPHeaderSize - input.SecurityOverhead - input.ProbePrefixSize,
		MinimumPayload:   minimumPayload,
		Attempts:         input.Attempts,
	}, nil
}

func RawUDPPathMTUFromPayload(payloadSize int, outerAddress netip.Addr) (int, error) {
	return DatagramPathMTUFromPayload(payloadSize, outerAddress, 0)
}

func DatagramPathMTUFromPayload(payloadSize int, outerAddress netip.Addr, securityOverhead int) (int, error) {
	if payloadSize < ProbeHeaderSize {
		return 0, fmt.Errorf("confirmed UDP payload must be at least %d", ProbeHeaderSize)
	}
	if !outerAddress.IsValid() {
		return 0, fmt.Errorf("outer address is required")
	}
	if securityOverhead < 0 {
		return 0, fmt.Errorf("security overhead must not be negative")
	}
	ipHeader := IPv4HeaderSize
	if outerAddress.Is6() {
		ipHeader = IPv6HeaderSize
	}
	return payloadSize + ipHeader + UDPHeaderSize + securityOverhead, nil
}

// PlanDatagram converts a peer-confirmed outer path MTU into fixed worker
// limits. It performs no probing and is therefore safe to use only after the
// control plane has confirmed PathMTU with the peer.
func PlanDatagram(input DatagramInput) (DatagramPlan, error) {
	if input.PathMTU <= 0 {
		return DatagramPlan{}, fmt.Errorf("path MTU must be positive")
	}
	if input.DeviceMTUCeiling <= 0 {
		return DatagramPlan{}, fmt.Errorf("device MTU ceiling must be positive")
	}
	if !input.OuterAddress.IsValid() {
		return DatagramPlan{}, fmt.Errorf("outer address is required")
	}
	if input.VKeyLength < 0 {
		return DatagramPlan{}, fmt.Errorf("vKey length must not be negative")
	}
	if input.SecurityOverhead < 0 {
		return DatagramPlan{}, fmt.Errorf("security overhead must not be negative")
	}

	ipHeader := IPv4HeaderSize
	if input.OuterAddress.Is6() {
		ipHeader = IPv6HeaderSize
	}
	controlOverhead := SegmentHeaderSize
	if input.VKeyLength > 0 {
		controlOverhead += VKeyHeaderBaseSize + input.VKeyLength
	}
	frameOverhead := 0
	if input.TAP {
		frameOverhead = EthernetHeaderSize
	}
	transportOverhead := UDPHeaderSize + input.SecurityOverhead
	maxWirePayload := input.PathMTU - ipHeader - transportOverhead
	maxSegmentFrameSize := maxWirePayload - controlOverhead
	deviceFrameCeiling := input.DeviceMTUCeiling + frameOverhead
	if maxSegmentFrameSize > deviceFrameCeiling {
		maxSegmentFrameSize = deviceFrameCeiling
	}
	effectiveNetworkMTU := maxSegmentFrameSize - frameOverhead
	if effectiveNetworkMTU <= IPv6HeaderSize+TCPHeaderSize {
		return DatagramPlan{}, fmt.Errorf("path MTU %d leaves no usable tunneled payload", input.PathMTU)
	}

	return DatagramPlan{
		PathMTU:             input.PathMTU,
		OuterIPHeader:       ipHeader,
		TransportOverhead:   transportOverhead,
		ControlOverhead:     controlOverhead,
		FrameOverhead:       frameOverhead,
		MaxWirePayload:      maxWirePayload,
		MaxFrameSize:        deviceFrameCeiling,
		MaxSegmentFrameSize: maxSegmentFrameSize,
		EffectiveNetworkMTU: effectiveNetworkMTU,
		TCPMSSIPv4:          effectiveNetworkMTU - IPv4HeaderSize - TCPHeaderSize,
		TCPMSSIPv6:          effectiveNetworkMTU - IPv6HeaderSize - TCPHeaderSize,
	}, nil
}
