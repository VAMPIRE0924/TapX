package config

import (
	"net/netip"
	"testing"

	"tapx/internal/pathmtu"
)

func TestCompileConfirmedUDPPathUpdatesGeneratedRuntimeOnly(t *testing.T) {
	runtime := &GeneratedRuntime{UDPPipes: []RuntimeUDPPipe{{
		EndpointKind: "connector", EndpointID: "connector-a", DeviceID: "tun-a",
		MaxFrameSize: 1500, LinkAutoOptimize: true,
	}}}
	plan, err := pathmtu.PlanDatagram(pathmtu.DatagramInput{
		PathMTU: 1380, DeviceMTUCeiling: 1500, OuterAddress: netip.MustParseAddr("203.0.113.10"),
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmation := pathmtu.ConfirmedPath{
		Key:  pathmtu.PathKey{EndpointKind: "connector", EndpointID: "connector-a", DeviceID: "tun-a"},
		Plan: plan,
	}
	if err := CompileConfirmedUDPPath(runtime, confirmation); err != nil {
		t.Fatal(err)
	}
	pipe := runtime.UDPPipes[0]
	if pipe.MaxFrameSize != 1500 || pipe.MaxDatagramPayload != 1352 || pipe.ConfirmedPathMTU != 1380 ||
		pipe.EffectiveNetworkMTU != 1332 || pipe.TCPMSSIPv4 != 1292 || pipe.TCPMSSIPv6 != 1272 {
		t.Fatalf("compiled pipe = %+v", pipe)
	}
}

func TestCompileConfirmedUDPPathRejectsDisabledOptimization(t *testing.T) {
	runtime := &GeneratedRuntime{UDPPipes: []RuntimeUDPPipe{{
		EndpointKind: "connector", EndpointID: "connector-a", DeviceID: "tun-a", MaxFrameSize: 1500,
	}}}
	plan, err := pathmtu.PlanDatagram(pathmtu.DatagramInput{
		PathMTU: 1380, DeviceMTUCeiling: 1500, OuterAddress: netip.MustParseAddr("203.0.113.10"),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = CompileConfirmedUDPPath(runtime, pathmtu.ConfirmedPath{
		Key:  pathmtu.PathKey{EndpointKind: "connector", EndpointID: "connector-a", DeviceID: "tun-a"},
		Plan: plan,
	})
	if err == nil {
		t.Fatal("disabled optimizer accepted a confirmed plan")
	}
}
