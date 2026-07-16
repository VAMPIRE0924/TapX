package pathmtu

import (
	"context"
	"fmt"
	"time"
)

type RawUDPConfirmationInput struct {
	Key               PathKey
	DeviceMTUCeiling  int
	MinimumNetworkMTU int
	TAP               bool
	VKeyLength        int
	ProbePrefixSize   int
	SecurityOverhead  int
	Attempts          int
}

type Confirmer struct {
	Cache  *Cache
	Runner CommandRunner
	Now    func() time.Time
}

type ProbeCommitExchanger interface {
	Exchange(context.Context, []byte) ([]byte, error)
	Commit(context.Context, int, int) error
}

// ConfirmRawUDP resolves the current Linux route, obtains an exact peer
// acknowledgement, compiles immutable worker limits, and only then replaces
// the cached plan. A failed replacement leaves the previous plan untouched.
func (c *Confirmer) ConfirmRawUDP(ctx context.Context, input RawUDPConfirmationInput, exchange ProbeExchange) (ConfirmedPath, error) {
	entry, err := c.confirmRawUDP(ctx, input, exchange)
	if err != nil {
		return ConfirmedPath{}, err
	}
	c.store(entry)
	return entry, nil
}

func (c *Confirmer) ConfirmAndCommitRawUDP(ctx context.Context, input RawUDPConfirmationInput, exchanger ProbeCommitExchanger) (ConfirmedPath, error) {
	if exchanger == nil {
		return ConfirmedPath{}, fmt.Errorf("UDP exchanger is required")
	}
	entry, err := c.confirmRawUDP(ctx, input, exchanger.Exchange)
	if err != nil {
		return ConfirmedPath{}, err
	}
	if err := exchanger.Commit(ctx, entry.Probe.PayloadSize, input.Attempts); err != nil {
		return ConfirmedPath{}, err
	}
	c.store(entry)
	return entry, nil
}

func (c *Confirmer) AcceptRawUDPCommit(ctx context.Context, input RawUDPConfirmationInput, committed UDPConfirmedPath) (ConfirmedPath, error) {
	if !committed.Peer.IsValid() || committed.PayloadSize < ProbeHeaderSize {
		return ConfirmedPath{}, fmt.Errorf("committed UDP path is incomplete")
	}
	input.Key.Remote = committed.Peer
	if err := validateRawUDPConfirmationInput(input); err != nil {
		return ConfirmedPath{}, err
	}
	route, err := DiscoverRouteCandidate(ctx, committed.Peer.Addr(), c.runner())
	if err != nil {
		return ConfirmedPath{}, err
	}
	entry, err := c.compileRawUDPConfirmation(input, route, ConfirmResult{PayloadSize: committed.PayloadSize})
	if err != nil {
		return ConfirmedPath{}, err
	}
	c.store(entry)
	return entry, nil
}

func (c *Confirmer) confirmRawUDP(ctx context.Context, input RawUDPConfirmationInput, exchange ProbeExchange) (ConfirmedPath, error) {
	if err := validateRawUDPConfirmationInput(input); err != nil {
		return ConfirmedPath{}, err
	}
	route, err := DiscoverRouteCandidate(ctx, input.Key.Remote.Addr(), c.runner())
	if err != nil {
		return ConfirmedPath{}, err
	}
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling:  input.DeviceMTUCeiling,
		RouteMTUCandidate: route.MTU,
		MinimumNetworkMTU: input.MinimumNetworkMTU,
		OuterAddress:      input.Key.Remote.Addr(),
		TAP:               input.TAP,
		VKeyLength:        input.VKeyLength,
		ProbePrefixSize:   input.ProbePrefixSize,
		SecurityOverhead:  input.SecurityOverhead,
		Attempts:          input.Attempts,
	})
	if err != nil {
		return ConfirmedPath{}, err
	}
	probe, err := ConfirmPayload(ctx, options, exchange)
	if err != nil {
		return ConfirmedPath{}, err
	}
	return c.compileRawUDPConfirmation(input, route, probe)
}

func validateRawUDPConfirmationInput(input RawUDPConfirmationInput) error {
	if input.Key.EndpointKind == "" || input.Key.EndpointID == "" || input.Key.DeviceID == "" || input.Key.Transport == "" {
		return fmt.Errorf("path key requires endpoint kind, endpoint ID, device ID, and transport")
	}
	if !input.Key.Remote.IsValid() {
		return fmt.Errorf("path key remote address is required")
	}
	if input.SecurityOverhead < 0 {
		return fmt.Errorf("security overhead must not be negative")
	}
	if input.ProbePrefixSize < 0 {
		return fmt.Errorf("probe prefix size must not be negative")
	}
	return nil
}

func (c *Confirmer) compileRawUDPConfirmation(input RawUDPConfirmationInput, route RouteCandidate, probe ConfirmResult) (ConfirmedPath, error) {
	confirmedMTU, err := DatagramPathMTUFromPayload(probe.PayloadSize+input.ProbePrefixSize, input.Key.Remote.Addr(), input.SecurityOverhead)
	if err != nil {
		return ConfirmedPath{}, err
	}
	plan, err := PlanDatagram(DatagramInput{
		PathMTU:          confirmedMTU,
		DeviceMTUCeiling: input.DeviceMTUCeiling,
		OuterAddress:     input.Key.Remote.Addr(),
		TAP:              input.TAP,
		VKeyLength:       input.VKeyLength,
		SecurityOverhead: input.SecurityOverhead,
	})
	if err != nil {
		return ConfirmedPath{}, err
	}
	now := time.Now
	if c != nil && c.Now != nil {
		now = c.Now
	}
	entry := ConfirmedPath{
		Key:         input.Key,
		Route:       route,
		Probe:       probe,
		Plan:        plan,
		ConfirmedAt: now().UTC(),
	}
	return entry, nil
}

func (c *Confirmer) runner() CommandRunner {
	if c == nil {
		return nil
	}
	return c.Runner
}

func (c *Confirmer) store(entry ConfirmedPath) {
	if c != nil && c.Cache != nil {
		c.Cache.Store(entry)
	}
}
