package panel

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tapx/internal/config"
)

type ClientTrafficReset struct {
	ClientID        string        `json:"clientId"`
	ResetAt         int64         `json:"resetAt"`
	TrafficRXOffset uint64        `json:"trafficRxOffset"`
	TrafficTXOffset uint64        `json:"trafficTxOffset"`
	Counters        StatsCounters `json:"counters"`
}

func (s *Server) handleClientTraffic(w http.ResponseWriter, r *http.Request) {
	id, action, err := clientTrafficPath(strings.TrimPrefix(r.URL.Path, "/api/clients/"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if action != "traffic/reset" {
		writeErrorStatus(w, http.StatusNotFound, fmt.Errorf("unknown client traffic action"))
		return
	}
	cfg, reset, err := resetClientTraffic(r.Context(), s.store, s.runtime.State(), id, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "client.traffic.reset", fmt.Sprintf("%s reset to rx=%d tx=%d", id, reset.TrafficRXOffset, reset.TrafficTXOffset))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg, "reset": reset})
}

func clientTrafficPath(path string) (id string, action string, err error) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("client traffic path must be /api/clients/{id}/traffic/reset")
	}
	if parts[0] == "" {
		return "", "", fmt.Errorf("client id is required")
	}
	return parts[0], strings.Join(parts[1:], "/"), nil
}

func resetClientTraffic(ctx context.Context, store *Store, state RuntimeState, id string, now time.Time) (config.RuntimeConfig, ClientTrafficReset, error) {
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return config.RuntimeConfig{}, ClientTrafficReset{}, err
	}
	raw := clientRawCountersFromRuntimeState(state, id)
	reset := ClientTrafficReset{
		ClientID:        id,
		ResetAt:         now.Unix(),
		TrafficRXOffset: raw.RXBytes,
		TrafficTXOffset: raw.TXBytes,
		Counters:        raw,
	}
	found := false
	for i := range cfg.Clients {
		if cfg.Clients[i].ID != id {
			continue
		}
		cfg.Clients[i].TrafficResetAt = reset.ResetAt
		cfg.Clients[i].TrafficRXOffset = reset.TrafficRXOffset
		cfg.Clients[i].TrafficTXOffset = reset.TrafficTXOffset
		found = true
		break
	}
	if !found {
		return config.RuntimeConfig{}, ClientTrafficReset{}, ErrNotFound
	}
	if err := store.ReplaceConfig(ctx, cfg); err != nil {
		return config.RuntimeConfig{}, ClientTrafficReset{}, err
	}
	return cfg, reset, nil
}

func clientRawCountersFromRuntimeState(state RuntimeState, clientID string) StatsCounters {
	var out StatsCounters
	add := func(pipe RuntimePipeState) {
		if pipe.ClientID != clientID {
			return
		}
		counters := countersFromFastpath(pipe.Counters)
		out.add(counters)
	}
	for _, pipe := range state.UDPPipes {
		add(pipe)
	}
	for _, pipe := range state.TCPPipes {
		add(pipe)
	}
	for _, pipe := range state.XrayPipes {
		add(pipe)
	}
	return out
}
