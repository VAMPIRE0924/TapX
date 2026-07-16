package panel

import (
	"context"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
)

func refreshLimitConfig(store *Store, state RuntimeState, now time.Time) (config.RuntimeConfig, error) {
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	now = now.In(settingsLocation(cfg.Settings))
	changed := false
	for i := range cfg.Listeners {
		listener := &cfg.Listeners[i]
		if !trafficResetDue(listener.TrafficReset, listener.TrafficResetAt, now) {
			continue
		}
		counters := endpointRawCountersFromRuntimeState(state, "listener", listener.ID)
		listener.TrafficResetAt = now.Unix()
		listener.TrafficResetGeneration = state.Generation
		listener.TrafficRXOffset = counters.RXBytes
		listener.TrafficTXOffset = counters.TXBytes
		changed = true
	}
	for i := range cfg.Clients {
		client := &cfg.Clients[i]
		if client.ExpiresAt < 0 && clientHasTraffic(state, client.ID) {
			client.ExpiresAt = now.Unix() - client.ExpiresAt
			changed = true
		}
		if !trafficResetDue(client.TrafficReset, client.TrafficResetAt, now) {
			continue
		}
		counters := clientRawCountersFromRuntimeState(state, client.ID)
		client.TrafficResetAt = now.Unix()
		client.TrafficResetGeneration = state.Generation
		client.TrafficRXOffset = counters.RXBytes
		client.TrafficTXOffset = counters.TXBytes
		changed = true
	}
	if changed {
		if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
			return config.RuntimeConfig{}, err
		}
	}
	return cfg, nil
}

func settingsLocation(settings []model.Settings) *time.Location {
	for _, item := range settings {
		if !item.Enabled || item.Timezone == "" {
			continue
		}
		if location, err := time.LoadLocation(item.Timezone); err == nil {
			return location
		}
	}
	return time.Local
}

func clientHasTraffic(state RuntimeState, clientID string) bool {
	if clientID == "" {
		return false
	}
	for _, pipes := range [][]RuntimePipeState{state.UDPPipes, state.TCPPipes, state.XrayPipes} {
		for _, pipe := range pipes {
			if pipe.ClientID != clientID {
				continue
			}
			if pipe.Counters.RXPackets > 0 || pipe.Counters.TXPackets > 0 || pipe.Counters.RXBytes > 0 || pipe.Counters.TXBytes > 0 {
				return true
			}
		}
	}
	return false
}

func trafficResetDue(mode string, lastReset int64, now time.Time) bool {
	var boundary time.Time
	switch mode {
	case "hourly":
		boundary = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	case "daily":
		boundary = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "weekly":
		weekday := (int(now.Weekday()) + 6) % 7
		start := now.AddDate(0, 0, -weekday)
		boundary = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, now.Location())
	case "monthly":
		boundary = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	default:
		return false
	}
	return lastReset < boundary.Unix()
}
