package panel

import (
	"sort"
	"time"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
)

type StatsReport struct {
	GeneratedAt string             `json:"generatedAt"`
	Runtime     RuntimeState       `json:"runtime"`
	Totals      StatsCounters      `json:"totals"`
	ByTransport []StatsBucket      `json:"byTransport"`
	ByDevice    []StatsBucket      `json:"byDevice"`
	ByRoute     []StatsBucket      `json:"byRoute"`
	ByClient    []StatsBucket      `json:"byClient"`
	ByEndpoint  []StatsBucket      `json:"byEndpoint"`
	Clients     []ClientQuotaState `json:"clients"`
}

type StatsCounters struct {
	RXPackets  uint64 `json:"rxPackets"`
	TXPackets  uint64 `json:"txPackets"`
	RXBytes    uint64 `json:"rxBytes"`
	TXBytes    uint64 `json:"txBytes"`
	DropsGuard uint64 `json:"dropsGuard"`
	DropsIO    uint64 `json:"dropsIO"`
}

type StatsBucket struct {
	ID       string        `json:"id"`
	Name     string        `json:"name,omitempty"`
	Kind     string        `json:"kind,omitempty"`
	Endpoint string        `json:"endpoint,omitempty"`
	Pipes    int           `json:"pipes"`
	Counters StatsCounters `json:"counters"`
}

type ClientQuotaState struct {
	ID             string        `json:"id"`
	Name           string        `json:"name,omitempty"`
	Email          string        `json:"email,omitempty"`
	Enabled        bool          `json:"enabled"`
	ExpiresAt      int64         `json:"expiresAt,omitempty"`
	Expired        bool          `json:"expired"`
	TrafficCap     uint64        `json:"trafficCap,omitempty"`
	TrafficResetAt int64         `json:"trafficResetAt,omitempty"`
	UsedBytes      uint64        `json:"usedBytes"`
	RemainingBytes uint64        `json:"remainingBytes,omitempty"`
	OverQuota      bool          `json:"overQuota"`
	ActivePipes    int           `json:"activePipes"`
	Counters       StatsCounters `json:"counters"`
}

type ClientEnforcementPlanItem struct {
	ClientID string
	Reason   string
}

func BuildStatsReport(cfg config.RuntimeConfig, state RuntimeState, now time.Time) StatsReport {
	acc := &statsAccumulator{
		transport: map[string]*StatsBucket{},
		devices:   map[string]*StatsBucket{},
		routes:    map[string]*StatsBucket{},
		clients:   map[string]*StatsBucket{},
		endpoints: map[string]*StatsBucket{},
	}
	names := statsNames(cfg)
	for _, pipe := range state.UDPPipes {
		acc.addPipe(pipe, names)
	}
	for _, pipe := range state.TCPPipes {
		acc.addPipe(pipe, names)
	}
	for _, pipe := range state.XrayPipes {
		acc.addPipe(pipe, names)
	}

	clients := clientQuotaStates(cfg.Clients, acc.clients, now)
	adjustClientBuckets(acc.clients, cfg.Clients)
	return StatsReport{
		GeneratedAt: now.UTC().Format(time.RFC3339Nano),
		Runtime:     state,
		Totals:      acc.total,
		ByTransport: sortedBuckets(acc.transport),
		ByDevice:    sortedBuckets(acc.devices),
		ByRoute:     sortedBuckets(acc.routes),
		ByClient:    sortedBuckets(acc.clients),
		ByEndpoint:  sortedBuckets(acc.endpoints),
		Clients:     clients,
	}
}

func BuildClientEnforcementPlan(cfg config.RuntimeConfig, state RuntimeState, now time.Time) []ClientEnforcementPlanItem {
	report := BuildStatsReport(cfg, state, now)
	out := make([]ClientEnforcementPlanItem, 0)
	for _, client := range report.Clients {
		if client.ActivePipes == 0 {
			continue
		}
		reason := ""
		switch {
		case !client.Enabled:
			reason = "disabled"
		case client.Expired:
			reason = "expired"
		case client.OverQuota:
			reason = "quota"
		}
		if reason != "" {
			out = append(out, ClientEnforcementPlanItem{
				ClientID: client.ID,
				Reason:   reason,
			})
		}
	}
	return out
}

type statsAccumulator struct {
	total     StatsCounters
	transport map[string]*StatsBucket
	devices   map[string]*StatsBucket
	routes    map[string]*StatsBucket
	clients   map[string]*StatsBucket
	endpoints map[string]*StatsBucket
}

type statsNameIndex struct {
	devices map[string]string
	routes  map[string]string
	clients map[string]model.Client
}

func statsNames(cfg config.RuntimeConfig) statsNameIndex {
	out := statsNameIndex{
		devices: map[string]string{},
		routes:  map[string]string{},
		clients: map[string]model.Client{},
	}
	for _, item := range cfg.Devices {
		out.devices[item.ID] = firstNonEmpty(item.Name, item.IfName)
	}
	for _, item := range cfg.Routes {
		out.routes[item.ID] = item.ID
	}
	for _, item := range cfg.Clients {
		out.clients[item.ID] = item
	}
	return out
}

func (a *statsAccumulator) addPipe(pipe RuntimePipeState, names statsNameIndex) {
	counters := countersFromFastpath(pipe.Counters)
	a.total.add(counters)
	a.bucket(a.transport, pipe.Transport, pipe.Transport, "", "", counters)
	a.bucket(a.devices, pipe.DeviceID, names.devices[pipe.DeviceID], "", "", counters)
	if pipe.RouteID != "" {
		a.bucket(a.routes, pipe.RouteID, names.routes[pipe.RouteID], "", "", counters)
	}
	if pipe.ClientID != "" {
		client := names.clients[pipe.ClientID]
		a.bucket(a.clients, pipe.ClientID, firstNonEmpty(client.Name, client.Email), "", "", counters)
	}
	endpointID := pipe.EndpointKind + ":" + pipe.EndpointID
	a.bucket(a.endpoints, endpointID, pipe.EndpointID, pipe.EndpointKind, pipe.Transport, counters)
}

func (a *statsAccumulator) bucket(index map[string]*StatsBucket, id, name, kind, endpoint string, counters StatsCounters) {
	if id == "" {
		id = "(unbound)"
	}
	bucket := index[id]
	if bucket == nil {
		bucket = &StatsBucket{ID: id, Name: name, Kind: kind, Endpoint: endpoint}
		index[id] = bucket
	}
	bucket.Pipes++
	bucket.Counters.add(counters)
}

func countersFromFastpath(in fastpath.CountersSnapshot) StatsCounters {
	return StatsCounters{
		RXPackets:  in.RXPackets,
		TXPackets:  in.TXPackets,
		RXBytes:    in.RXBytes,
		TXBytes:    in.TXBytes,
		DropsGuard: in.DropsGuard,
		DropsIO:    in.DropsIO,
	}
}

func (c *StatsCounters) add(next StatsCounters) {
	c.RXPackets += next.RXPackets
	c.TXPackets += next.TXPackets
	c.RXBytes += next.RXBytes
	c.TXBytes += next.TXBytes
	c.DropsGuard += next.DropsGuard
	c.DropsIO += next.DropsIO
}

func sortedBuckets(index map[string]*StatsBucket) []StatsBucket {
	out := make([]StatsBucket, 0, len(index))
	for _, bucket := range index {
		out = append(out, *bucket)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func clientQuotaStates(clients []model.Client, buckets map[string]*StatsBucket, now time.Time) []ClientQuotaState {
	out := make([]ClientQuotaState, 0, len(clients))
	unixNow := now.Unix()
	for _, client := range clients {
		var counters StatsCounters
		activePipes := 0
		if bucket := buckets[client.ID]; bucket != nil {
			counters = bucket.Counters
			activePipes = bucket.Pipes
		}
		counters = adjustClientCounters(client, counters)
		used := counters.RXBytes + counters.TXBytes
		state := ClientQuotaState{
			ID:             client.ID,
			Name:           client.Name,
			Email:          client.Email,
			Enabled:        client.Enabled,
			ExpiresAt:      client.ExpiresAt,
			Expired:        client.ExpiresAt > 0 && client.ExpiresAt <= unixNow,
			TrafficCap:     client.TrafficCap,
			TrafficResetAt: client.TrafficResetAt,
			UsedBytes:      used,
			ActivePipes:    activePipes,
			Counters:       counters,
		}
		if client.TrafficCap > 0 {
			if used >= client.TrafficCap {
				state.OverQuota = true
			} else {
				state.RemainingBytes = client.TrafficCap - used
			}
		}
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func adjustClientBuckets(buckets map[string]*StatsBucket, clients []model.Client) {
	for _, client := range clients {
		bucket := buckets[client.ID]
		if bucket == nil {
			continue
		}
		bucket.Counters = adjustClientCounters(client, bucket.Counters)
	}
}

func adjustClientCounters(client model.Client, counters StatsCounters) StatsCounters {
	counters.RXBytes = subtractCounterOffset(counters.RXBytes, client.TrafficRXOffset)
	counters.TXBytes = subtractCounterOffset(counters.TXBytes, client.TrafficTXOffset)
	return counters
}

func subtractCounterOffset(value, offset uint64) uint64 {
	if value <= offset {
		return 0
	}
	return value - offset
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
