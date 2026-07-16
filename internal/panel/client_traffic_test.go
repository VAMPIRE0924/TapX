package panel

import (
	"context"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
)

func TestResetClientTrafficStoresCurrentRuntimeOffsets(t *testing.T) {
	store := newTestStore(t)
	cfg := config.RuntimeConfig{
		Clients: []model.Client{{ID: "client-a", Enabled: true, TrafficCap: 1000}},
	}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	state := RuntimeState{
		Generation: 4,
		UDPPipes: []RuntimePipeState{{
			EndpointID:   "udp-a",
			EndpointKind: "listener",
			Transport:    "udp",
			ClientID:     "client-a",
			Counters: fastpath.CountersSnapshot{
				RXBytes: 400,
				TXBytes: 200,
			},
		}},
	}
	now := time.Unix(2500, 0)

	next, reset, err := resetClientTraffic(context.Background(), store, state, "client-a", now)
	if err != nil {
		t.Fatalf("reset client traffic: %v", err)
	}
	if reset.TrafficRXOffset != 400 || reset.TrafficTXOffset != 200 || reset.ResetAt != now.Unix() || reset.Generation != 4 {
		t.Fatalf("reset = %+v, want current counters", reset)
	}
	if next.Clients[0].TrafficRXOffset != 400 || next.Clients[0].TrafficTXOffset != 200 || next.Clients[0].TrafficResetGeneration != 4 {
		t.Fatalf("stored client = %+v, want offsets", next.Clients[0])
	}

	report := BuildStatsReport(next, state, now)
	if report.Clients[0].UsedBytes != 0 || report.Clients[0].OverQuota {
		t.Fatalf("client quota after reset = %+v, want zero used", report.Clients[0])
	}
}
