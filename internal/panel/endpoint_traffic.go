package panel

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tapx/internal/config"
)

type EndpointTrafficReset struct {
	EndpointID      string        `json:"endpointId"`
	EndpointKind    string        `json:"endpointKind"`
	ResetAt         int64         `json:"resetAt"`
	Generation      uint64        `json:"generation"`
	TrafficRXOffset uint64        `json:"trafficRxOffset"`
	TrafficTXOffset uint64        `json:"trafficTxOffset"`
	Counters        StatsCounters `json:"counters"`
}

func (s *Server) handleConnectorTraffic(w http.ResponseWriter, r *http.Request) {
	s.handleEndpointTraffic(w, r, "connector", strings.TrimPrefix(r.URL.Path, "/api/connectors/"))
}

func (s *Server) handleListenerTraffic(w http.ResponseWriter, r *http.Request) {
	s.handleEndpointTraffic(w, r, "listener", strings.TrimPrefix(r.URL.Path, "/api/listeners/"))
}

func (s *Server) handleEndpointTraffic(w http.ResponseWriter, r *http.Request, kind, path string) {
	id, action, err := endpointTrafficPath(path)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if action != "traffic/reset" {
		writeErrorStatus(w, http.StatusNotFound, fmt.Errorf("unknown %s traffic action", kind))
		return
	}
	cfg, reset, err := resetEndpointTraffic(r.Context(), s.store, s.runtime.State(), kind, id, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	s.log("info", kind+".traffic.reset", fmt.Sprintf("%s reset to rx=%d tx=%d generation=%d", id, reset.TrafficRXOffset, reset.TrafficTXOffset, reset.Generation))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg, "reset": reset})
}

func endpointTrafficPath(path string) (id, action string, err error) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] == "" {
		return "", "", fmt.Errorf("endpoint traffic path must be /api/{listeners|connectors}/{id}/traffic/reset")
	}
	return parts[0], strings.Join(parts[1:], "/"), nil
}

func resetEndpointTraffic(ctx context.Context, store *Store, state RuntimeState, kind, id string, now time.Time) (config.RuntimeConfig, EndpointTrafficReset, error) {
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return config.RuntimeConfig{}, EndpointTrafficReset{}, err
	}
	raw := endpointRawCountersFromRuntimeState(state, kind, id)
	reset := EndpointTrafficReset{
		EndpointID:      id,
		EndpointKind:    kind,
		ResetAt:         now.Unix(),
		Generation:      state.Generation,
		TrafficRXOffset: raw.RXBytes,
		TrafficTXOffset: raw.TXBytes,
		Counters:        raw,
	}
	found := false
	switch kind {
	case "listener":
		for i := range cfg.Listeners {
			if cfg.Listeners[i].ID != id {
				continue
			}
			cfg.Listeners[i].TrafficResetAt = reset.ResetAt
			cfg.Listeners[i].TrafficResetGeneration = reset.Generation
			cfg.Listeners[i].TrafficRXOffset = reset.TrafficRXOffset
			cfg.Listeners[i].TrafficTXOffset = reset.TrafficTXOffset
			found = true
			break
		}
	case "connector":
		for i := range cfg.Connectors {
			if cfg.Connectors[i].ID != id {
				continue
			}
			cfg.Connectors[i].TrafficResetAt = reset.ResetAt
			cfg.Connectors[i].TrafficResetGeneration = reset.Generation
			cfg.Connectors[i].TrafficRXOffset = reset.TrafficRXOffset
			cfg.Connectors[i].TrafficTXOffset = reset.TrafficTXOffset
			found = true
			break
		}
	default:
		return config.RuntimeConfig{}, EndpointTrafficReset{}, fmt.Errorf("unsupported endpoint kind %q", kind)
	}
	if !found {
		return config.RuntimeConfig{}, EndpointTrafficReset{}, ErrNotFound
	}
	if err := store.ReplaceConfig(ctx, cfg); err != nil {
		return config.RuntimeConfig{}, EndpointTrafficReset{}, err
	}
	return cfg, reset, nil
}

func endpointRawCountersFromRuntimeState(state RuntimeState, kind, id string) StatsCounters {
	var out StatsCounters
	add := func(pipe RuntimePipeState) {
		if pipe.EndpointKind != kind || pipe.EndpointID != id {
			return
		}
		out.add(countersFromFastpath(pipe.Counters))
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
