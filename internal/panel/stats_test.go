package panel

import (
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
)

func TestBuildStatsReportAggregatesRuntimeCountersAndClientQuota(t *testing.T) {
	now := time.Unix(2000, 0)
	cfg := config.RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Name: "Tunnel A", IfName: "tapx0"}},
		Clients: []model.Client{
			{ID: "client-a", Enabled: true, Name: "Alice", TrafficCap: 1000, ExpiresAt: 3000},
			{ID: "client-b", Enabled: true, Name: "Bob", TrafficCap: 100, ExpiresAt: 1000},
		},
	}
	state := RuntimeState{
		Running: true,
		UDPPipes: []RuntimePipeState{{
			EndpointID:   "udp-a",
			EndpointKind: "listener",
			Transport:    "udp",
			RouteID:      "route-a",
			DeviceID:     "tun-a",
			ClientID:     "client-a",
			Counters: fastpath.CountersSnapshot{
				RXPackets: 2,
				TXPackets: 3,
				RXBytes:   400,
				TXBytes:   200,
			},
		}},
		TCPPipes: []RuntimePipeState{{
			EndpointID:   "tcp-a",
			EndpointKind: "connector",
			Transport:    "tcp",
			DeviceID:     "tun-a",
			ClientID:     "client-b",
			Counters: fastpath.CountersSnapshot{
				RXPackets: 1,
				TXPackets: 1,
				RXBytes:   80,
				TXBytes:   70,
				DropsIO:   1,
			},
		}},
	}

	report := BuildStatsReport(cfg, state, now)
	if report.Totals.RXPackets != 3 || report.Totals.TXBytes != 270 || report.Totals.DropsIO != 1 {
		t.Fatalf("totals = %+v, want summed counters", report.Totals)
	}
	if len(report.ByTransport) != 2 {
		t.Fatalf("by transport = %+v, want udp/tcp", report.ByTransport)
	}
	if len(report.ByDevice) != 1 || report.ByDevice[0].ID != "tun-a" || report.ByDevice[0].Counters.RXBytes != 480 {
		t.Fatalf("by device = %+v, want tun-a aggregate", report.ByDevice)
	}
	if len(report.Clients) != 2 {
		t.Fatalf("clients = %+v, want 2", report.Clients)
	}
	if report.Clients[0].ID != "client-a" || report.Clients[0].UsedBytes != 600 || report.Clients[0].RemainingBytes != 400 || report.Clients[0].OverQuota {
		t.Fatalf("client-a quota = %+v", report.Clients[0])
	}
	if report.Clients[1].ID != "client-b" || report.Clients[1].UsedBytes != 150 || !report.Clients[1].OverQuota || !report.Clients[1].Expired {
		t.Fatalf("client-b quota = %+v", report.Clients[1])
	}
}

func TestBuildStatsReportAppliesClientTrafficResetOffsets(t *testing.T) {
	now := time.Unix(2000, 0)
	cfg := config.RuntimeConfig{
		Clients: []model.Client{{
			ID:              "client-a",
			Enabled:         true,
			TrafficCap:      100,
			TrafficResetAt:  1900,
			TrafficRXOffset: 30,
			TrafficTXOffset: 20,
		}},
	}
	state := RuntimeState{
		UDPPipes: []RuntimePipeState{{
			EndpointID:   "udp-a",
			EndpointKind: "listener",
			Transport:    "udp",
			ClientID:     "client-a",
			Counters: fastpath.CountersSnapshot{
				RXBytes: 70,
				TXBytes: 40,
			},
		}},
	}

	report := BuildStatsReport(cfg, state, now)
	if report.Clients[0].UsedBytes != 60 || report.Clients[0].RemainingBytes != 40 || report.Clients[0].TrafficResetAt != 1900 {
		t.Fatalf("client quota = %+v, want adjusted reset usage", report.Clients[0])
	}
	if len(report.ByClient) != 1 || report.ByClient[0].Counters.RXBytes != 40 || report.ByClient[0].Counters.TXBytes != 20 {
		t.Fatalf("by client = %+v, want adjusted counters", report.ByClient)
	}
}

func TestBuildClientEnforcementPlan(t *testing.T) {
	now := time.Unix(2000, 0)
	cfg := config.RuntimeConfig{
		Clients: []model.Client{
			{ID: "disabled", Enabled: false},
			{ID: "expired", Enabled: true, ExpiresAt: 1000},
			{ID: "quota", Enabled: true, TrafficCap: 100},
			{ID: "ok", Enabled: true, TrafficCap: 1000, ExpiresAt: 3000},
		},
	}
	state := RuntimeState{
		UDPPipes: []RuntimePipeState{
			{EndpointID: "a", EndpointKind: "listener", Transport: "udp", ClientID: "disabled"},
			{EndpointID: "b", EndpointKind: "listener", Transport: "udp", ClientID: "expired"},
			{EndpointID: "c", EndpointKind: "listener", Transport: "udp", ClientID: "quota", Counters: fastpath.CountersSnapshot{RXBytes: 60, TXBytes: 40}},
			{EndpointID: "d", EndpointKind: "listener", Transport: "udp", ClientID: "ok", Counters: fastpath.CountersSnapshot{RXBytes: 10}},
		},
	}

	plan := BuildClientEnforcementPlan(cfg, state, now)
	if len(plan) != 3 {
		t.Fatalf("plan = %+v, want 3 enforcement items", plan)
	}
	want := map[string]string{"disabled": "disabled", "expired": "expired", "quota": "quota"}
	for _, item := range plan {
		if want[item.ClientID] != item.Reason {
			t.Fatalf("plan item = %+v, want reason %q", item, want[item.ClientID])
		}
		delete(want, item.ClientID)
	}
	if len(want) != 0 {
		t.Fatalf("missing enforcement items: %+v", want)
	}
}
