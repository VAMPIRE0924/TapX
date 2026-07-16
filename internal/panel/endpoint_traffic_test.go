package panel

import (
	"context"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
)

func TestResetConnectorTrafficStoresRuntimeGenerationAndOffsets(t *testing.T) {
	store := newTestStore(t)
	cfg := config.RuntimeConfig{Connectors: []model.Connector{{
		ID: "connector-a", Enabled: true, Transport: model.TransportUDP, Remote: "127.0.0.1", Port: 9000,
	}}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	state := RuntimeState{
		Generation: 7,
		UDPPipes: []RuntimePipeState{
			{EndpointID: "connector-a", EndpointKind: "connector", Counters: fastpath.CountersSnapshot{RXBytes: 400, TXBytes: 200}},
			{EndpointID: "listener-a", EndpointKind: "listener", Counters: fastpath.CountersSnapshot{RXBytes: 999}},
		},
	}
	now := time.Unix(2500, 0)

	next, reset, err := resetEndpointTraffic(context.Background(), store, state, "connector", "connector-a", now)
	if err != nil {
		t.Fatalf("reset connector traffic: %v", err)
	}
	if reset.Generation != 7 || reset.TrafficRXOffset != 400 || reset.TrafficTXOffset != 200 {
		t.Fatalf("reset = %+v, want current connector counters", reset)
	}
	connector := next.Connectors[0]
	if connector.TrafficResetGeneration != 7 || connector.TrafficRXOffset != 400 || connector.TrafficTXOffset != 200 {
		t.Fatalf("stored connector = %+v, want generation and offsets", connector)
	}

	report := BuildStatsReport(next, state, now)
	if len(report.ByEndpoint) != 2 {
		t.Fatalf("by endpoint = %+v, want connector and listener", report.ByEndpoint)
	}
	for _, bucket := range report.ByEndpoint {
		if bucket.ID == "connector:connector-a" && (bucket.Counters.RXBytes != 0 || bucket.Counters.TXBytes != 0) {
			t.Fatalf("connector bucket = %+v, want reset counters", bucket)
		}
	}
}

func TestEndpointTrafficOffsetsDoNotLeakAcrossRuntimeGenerations(t *testing.T) {
	cfg := config.RuntimeConfig{Connectors: []model.Connector{{
		ID:                     "connector-a",
		TrafficResetGeneration: 6,
		TrafficRXOffset:        400,
		TrafficTXOffset:        200,
	}}}
	state := RuntimeState{
		Generation: 7,
		UDPPipes: []RuntimePipeState{{
			EndpointID: "connector-a", EndpointKind: "connector",
			Counters: fastpath.CountersSnapshot{RXBytes: 25, TXBytes: 15},
		}},
	}

	report := BuildStatsReport(cfg, state, time.Unix(3000, 0))
	if len(report.ByEndpoint) != 1 || report.ByEndpoint[0].Counters.RXBytes != 25 || report.ByEndpoint[0].Counters.TXBytes != 15 {
		t.Fatalf("by endpoint = %+v, want unadjusted new-generation counters", report.ByEndpoint)
	}
}
