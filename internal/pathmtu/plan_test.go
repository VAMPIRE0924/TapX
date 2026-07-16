package pathmtu

import (
	"context"
	"net/netip"
	"reflect"
	"testing"
)

func TestPlanDatagramRawUDPIPv4TUN(t *testing.T) {
	plan, err := PlanDatagram(DatagramInput{PathMTU: 1500, DeviceMTUCeiling: 1500, OuterAddress: netip.MustParseAddr("192.0.2.20")})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaxFrameSize != 1500 || plan.MaxSegmentFrameSize != 1452 || plan.EffectiveNetworkMTU != 1452 {
		t.Fatalf("plan = %+v, want 1500-byte frame ceiling and 1452-byte segment payload", plan)
	}
	if plan.TCPMSSIPv4 != 1412 || plan.TCPMSSIPv6 != 1392 {
		t.Fatalf("MSS = %d/%d, want 1412/1392", plan.TCPMSSIPv4, plan.TCPMSSIPv6)
	}
}

func TestPlanDatagramAccountsForTAPAndVKey(t *testing.T) {
	plan, err := PlanDatagram(DatagramInput{
		PathMTU: 1500, DeviceMTUCeiling: 1500, OuterAddress: netip.MustParseAddr("2001:db8::20"), TAP: true, VKeyLength: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ControlOverhead != 44 || plan.MaxFrameSize != 1514 || plan.MaxSegmentFrameSize != 1408 || plan.EffectiveNetworkMTU != 1394 {
		t.Fatalf("plan = %+v, want vKey and Ethernet overhead applied", plan)
	}
}

func TestPlanDatagramHonorsDeviceCeiling(t *testing.T) {
	plan, err := PlanDatagram(DatagramInput{
		PathMTU: 9000, DeviceMTUCeiling: 1400, OuterAddress: netip.MustParseAddr("198.51.100.10"), TAP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaxFrameSize != 1414 || plan.MaxSegmentFrameSize != 1414 || plan.EffectiveNetworkMTU != 1400 {
		t.Fatalf("plan = %+v, want device ceiling 1400", plan)
	}
}

func TestRawUDPConfirmOptionsUsesDeviceCeilingBeforeIPv4RouteCandidate(t *testing.T) {
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling: 1500, RouteMTUCandidate: 1500, MinimumNetworkMTU: 576,
		OuterAddress: netip.MustParseAddr("198.51.100.10"), Attempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := ConfirmOptions{DesiredPayload: 1520, CandidatePayload: 1472, MinimumPayload: 548, Attempts: 3}
	if options != want {
		t.Fatalf("options = %+v, want %+v", options, want)
	}
}

func TestRawUDPConfirmOptionsAccountsForIPv6TAPAndVKey(t *testing.T) {
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling: 1500, RouteMTUCandidate: 1500, MinimumNetworkMTU: 1280,
		OuterAddress: netip.MustParseAddr("2001:db8::10"), TAP: true, VKeyLength: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.DesiredPayload != 1558 || options.CandidatePayload != 1452 || options.MinimumPayload != 1232 {
		t.Fatalf("options = %+v", options)
	}
	pathMTU, err := RawUDPPathMTUFromPayload(options.CandidatePayload, netip.MustParseAddr("2001:db8::10"))
	if err != nil {
		t.Fatal(err)
	}
	if pathMTU != 1500 {
		t.Fatalf("path MTU = %d, want 1500", pathMTU)
	}
}

func TestRawUDPConfirmOptionsAccountsForProbePrefix(t *testing.T) {
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling: 1500, RouteMTUCandidate: 1500, MinimumNetworkMTU: 576,
		OuterAddress: netip.MustParseAddr("198.51.100.10"), VKeyLength: 5, ProbePrefixSize: 13,
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.DesiredPayload != 1520 || options.CandidatePayload != 1459 || options.MinimumPayload != 535 {
		t.Fatalf("options = %+v, want probe body sizes excluding the 13-byte vKey prefix", options)
	}
	pathMTU, err := RawUDPPathMTUFromPayload(options.CandidatePayload+13, netip.MustParseAddr("198.51.100.10"))
	if err != nil {
		t.Fatal(err)
	}
	if pathMTU != 1500 {
		t.Fatalf("path MTU = %d, want 1500", pathMTU)
	}
}

func TestRawUDPConfirmOptionsIPv6MinimumPathIncludesSecurityOverhead(t *testing.T) {
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling: 1500, RouteMTUCandidate: 1280, MinimumNetworkMTU: 1280,
		OuterAddress: netip.MustParseAddr("2001:db8::10"), SecurityOverhead: 37,
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.CandidatePayload != 1195 || options.MinimumPayload != 1195 {
		t.Fatalf("options = %+v, want a 1195-byte UDP payload for a 1280-byte IPv6 path", options)
	}
	pathMTU, err := DatagramPathMTUFromPayload(options.MinimumPayload, netip.MustParseAddr("2001:db8::10"), 37)
	if err != nil {
		t.Fatal(err)
	}
	if pathMTU != 1280 {
		t.Fatalf("path MTU = %d, want 1280", pathMTU)
	}
}

func TestRawUDPConfirmOptionsCapsMinimumAtDeviceCeiling(t *testing.T) {
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling: 900, RouteMTUCandidate: 1500, MinimumNetworkMTU: 1280,
		OuterAddress: netip.MustParseAddr("2001:db8::10"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.DesiredPayload != 920 || options.MinimumPayload != 920 {
		t.Fatalf("options = %+v, want the minimum capped at the useful device ceiling", options)
	}
}

func TestRawUDPConfirmOptionsSubtractsSecurityOverhead(t *testing.T) {
	options, err := RawUDPConfirmOptions(RawUDPProbeInput{
		DeviceMTUCeiling: 1500, RouteMTUCandidate: 1400, MinimumNetworkMTU: 576,
		OuterAddress: netip.MustParseAddr("192.0.2.10"), SecurityOverhead: 37,
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.CandidatePayload != 1335 {
		t.Fatalf("candidate payload = %d, want 1335", options.CandidatePayload)
	}
	pathMTU, err := DatagramPathMTUFromPayload(options.CandidatePayload, netip.MustParseAddr("192.0.2.10"), 37)
	if err != nil {
		t.Fatal(err)
	}
	if pathMTU != 1400 {
		t.Fatalf("path MTU = %d, want 1400", pathMTU)
	}
}

func TestDiscoverRouteCandidateUsesRouteMTU(t *testing.T) {
	var calls [][]string
	runner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte(`[{"dst":"203.0.113.9","dev":"eth0","prefsrc":"192.0.2.2","mtu":1420}]`), nil
	}
	candidate, err := DiscoverRouteCandidate(context.Background(), netip.MustParseAddr("203.0.113.9"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Device != "eth0" || candidate.MTU != 1420 || candidate.Source.String() != "192.0.2.2" {
		t.Fatalf("candidate = %+v", candidate)
	}
	want := [][]string{{"ip", "-json", "route", "get", "203.0.113.9"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestDiscoverRouteCandidateFallsBackToLinkMTU(t *testing.T) {
	var calls [][]string
	runner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if len(calls) == 1 {
			return []byte(`[{"dst":"203.0.113.9","dev":"wan0","prefsrc":"192.0.2.2"}]`), nil
		}
		return []byte(`[{"ifname":"wan0","mtu":1500}]`), nil
	}
	candidate, err := DiscoverRouteCandidate(context.Background(), netip.MustParseAddr("203.0.113.9"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.MTU != 1500 {
		t.Fatalf("candidate MTU = %d, want 1500", candidate.MTU)
	}
	want := [][]string{
		{"ip", "-json", "route", "get", "203.0.113.9"},
		{"ip", "-json", "link", "show", "dev", "wan0"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
