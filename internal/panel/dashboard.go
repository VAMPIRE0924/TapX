package panel

import (
	"net/http"
	"sync"
	"time"
)

const (
	dashboardRecentLogLimit = 8
	dashboardHistoryLimit   = 120
	defaultMetricLimit      = 720
	metricSampleInterval    = 5 * time.Second
)

type DashboardReport struct {
	GeneratedAt  string                  `json:"generatedAt"`
	Runtime      RuntimeState            `json:"runtime"`
	Stats        StatsReport             `json:"stats"`
	Rates        DashboardRates          `json:"rates"`
	Process      ProcessDiagnostic       `json:"process"`
	Fastpath     FastpathDiagnostic      `json:"fastpath"`
	OpenWrt      OpenWrtDiagnostic       `json:"openwrt"`
	System       SystemDiagnostic        `json:"system"`
	ObjectCounts map[string]int          `json:"objectCounts"`
	RecentLogs   []LogEvent              `json:"recentLogs"`
	History      []DashboardMetricSample `json:"history"`
}

type DashboardMetricSample struct {
	At                  int64   `json:"at"`
	CPU                 float64 `json:"cpu"`
	Memory              float64 `json:"memory"`
	Swap                float64 `json:"swap,omitempty"`
	DiskUsage           float64 `json:"diskUsage,omitempty"`
	EmbeddedXray        int     `json:"embeddedXray"`
	ExternalXray        int     `json:"externalXray"`
	TapX                int     `json:"tapx"`
	RX                  uint64  `json:"rx"`
	TX                  uint64  `json:"tx"`
	RXPackets           uint64  `json:"rxPackets,omitempty"`
	TXPackets           uint64  `json:"txPackets,omitempty"`
	DiskRead            uint64  `json:"diskRead,omitempty"`
	DiskWrite           uint64  `json:"diskWrite,omitempty"`
	TCPConnections      int     `json:"tcpConnections,omitempty"`
	UDPConnections      int     `json:"udpConnections,omitempty"`
	Online              int     `json:"online,omitempty"`
	Load1               float64 `json:"load1,omitempty"`
	Load5               float64 `json:"load5,omitempty"`
	Load15              float64 `json:"load15,omitempty"`
	Drops               uint64  `json:"drops"`
	TapXHeap            uint64  `json:"tapxHeap,omitempty"`
	TapXSys             uint64  `json:"tapxSys,omitempty"`
	TapXObjects         uint64  `json:"tapxObjects,omitempty"`
	TapXGC              uint32  `json:"tapxGC,omitempty"`
	TapXGCPause         uint64  `json:"tapxGCPause,omitempty"`
	TapXObservatory     int     `json:"tapxObservatory,omitempty"`
	EmbeddedHeap        uint64  `json:"embeddedHeap,omitempty"`
	EmbeddedSys         uint64  `json:"embeddedSys,omitempty"`
	EmbeddedObjects     uint64  `json:"embeddedObjects,omitempty"`
	EmbeddedGC          uint32  `json:"embeddedGC,omitempty"`
	EmbeddedGCPause     uint64  `json:"embeddedGCPause,omitempty"`
	EmbeddedObservatory int     `json:"embeddedObservatory,omitempty"`
	ExternalObservatory int     `json:"externalObservatory,omitempty"`
}

type DashboardRates struct {
	WindowSecond float64 `json:"windowSecond"`
	RXBytesPS    uint64  `json:"rxBytesPerSecond"`
	TXBytesPS    uint64  `json:"txBytesPerSecond"`
	RXPacketsPS  uint64  `json:"rxPacketsPerSecond"`
	TXPacketsPS  uint64  `json:"txPacketsPerSecond"`
	GuardDropsPS uint64  `json:"guardDropsPerSecond"`
	IODropsPS    uint64  `json:"ioDropsPerSecond"`
}

type dashboardRateTracker struct {
	mu     sync.Mutex
	at     time.Time
	totals StatsCounters
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	now := time.Now().UTC()
	state := s.runtime.State()
	stats := BuildStatsReport(cfg, state, now)
	diag := s.buildDiagnosticReport(cfg, now)
	system := s.system.Sample(stats, state)
	rates := s.dashboard.Rates(stats.Totals, now)
	report := DashboardReport{
		GeneratedAt:  now.Format(time.RFC3339Nano),
		Runtime:      state,
		Stats:        stats,
		Rates:        rates,
		Process:      diag.Process,
		Fastpath:     diag.Fastpath,
		OpenWrt:      diag.OpenWrt,
		System:       system,
		ObjectCounts: diag.ObjectCounts,
		RecentLogs:   recentLogEvents(s.logs.List(), dashboardRecentLogLimit),
	}
	sample := dashboardMetricSample(report, now)
	if err := s.store.AppendMetric(r.Context(), sample, metricSampleInterval, defaultMetricLimit); err != nil {
		s.log("warn", "metrics.persist", err.Error())
	} else if history, err := s.store.LoadMetrics(r.Context(), dashboardHistoryLimit); err != nil {
		s.log("warn", "metrics.load", err.Error())
	} else {
		report.History = history
	}
	writeJSON(w, http.StatusOK, report)
}

func dashboardMetricSample(report DashboardReport, now time.Time) DashboardMetricSample {
	embedded, external := 0, 0
	embeddedRunning := false
	for _, runtime := range report.Runtime.XrayRuntimes {
		switch runtime.Runtime {
		case "embedded":
			embedded += runtime.EndpointCount
			embeddedRunning = embeddedRunning || runtime.Running
		case "external":
			external += runtime.EndpointCount
		}
	}
	memory := float64(0)
	if report.System.MemoryTotal > 0 {
		memory = float64(report.System.MemoryUsed) * 100 / float64(report.System.MemoryTotal)
	}
	sample := DashboardMetricSample{
		At: now.UnixMilli(), CPU: report.System.CPUPercent, Memory: memory,
		EmbeddedXray: embedded, ExternalXray: external,
		TapX: len(report.Runtime.UDPPipes) + len(report.Runtime.TCPPipes),
		RX:   report.Rates.RXBytesPS, TX: report.Rates.TXBytesPS,
		RXPackets: report.Rates.RXPacketsPS, TXPackets: report.Rates.TXPacketsPS,
		DiskRead: report.System.DiskReadBPS, DiskWrite: report.System.DiskWriteBPS,
		TCPConnections: report.System.TCPConnections, UDPConnections: report.System.UDPConnections,
		Load1: report.System.Load1, Load5: report.System.Load5, Load15: report.System.Load15,
		Drops:    report.Stats.Totals.DropsGuard + report.Stats.Totals.DropsIO,
		TapXHeap: report.Process.HeapAlloc, TapXSys: report.Process.HeapSys,
		TapXObjects: report.Process.HeapObjects, TapXGC: report.Process.NumGC,
		TapXGCPause:         report.Process.LastGCPauseNs,
		TapXObservatory:     len(report.Runtime.UDPPipes) + len(report.Runtime.TCPPipes),
		EmbeddedObservatory: embedded, ExternalObservatory: external,
	}
	if report.System.SwapTotal > 0 {
		sample.Swap = float64(report.System.SwapUsed) * 100 / float64(report.System.SwapTotal)
	}
	if report.System.StorageTotal > 0 {
		sample.DiskUsage = float64(report.System.StorageUsed) * 100 / float64(report.System.StorageTotal)
	}
	for _, client := range report.Stats.Clients {
		if client.ActivePipes > 0 {
			sample.Online++
		}
	}
	if embeddedRunning {
		sample.EmbeddedHeap = report.Process.HeapAlloc
		sample.EmbeddedSys = report.Process.HeapSys
		sample.EmbeddedObjects = report.Process.HeapObjects
		sample.EmbeddedGC = report.Process.NumGC
		sample.EmbeddedGCPause = report.Process.LastGCPauseNs
	}
	return sample
}

func (t *dashboardRateTracker) Rates(current StatsCounters, now time.Time) DashboardRates {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.at.IsZero() {
		t.at = now
		t.totals = current
		return DashboardRates{}
	}
	elapsed := now.Sub(t.at).Seconds()
	previous := t.totals
	t.at = now
	t.totals = current
	if elapsed <= 0 {
		return DashboardRates{}
	}
	return DashboardRates{
		WindowSecond: elapsed,
		RXBytesPS:    counterRate(previous.RXBytes, current.RXBytes, elapsed),
		TXBytesPS:    counterRate(previous.TXBytes, current.TXBytes, elapsed),
		RXPacketsPS:  counterRate(previous.RXPackets, current.RXPackets, elapsed),
		TXPacketsPS:  counterRate(previous.TXPackets, current.TXPackets, elapsed),
		GuardDropsPS: counterRate(previous.DropsGuard, current.DropsGuard, elapsed),
		IODropsPS:    counterRate(previous.DropsIO, current.DropsIO, elapsed),
	}
}

func (t *dashboardRateTracker) Reset() {
	t.mu.Lock()
	t.at = time.Time{}
	t.totals = StatsCounters{}
	t.mu.Unlock()
}

func counterRate(previous, current uint64, seconds float64) uint64 {
	if current <= previous || seconds <= 0 {
		return 0
	}
	return uint64(float64(current-previous) / seconds)
}

func recentLogEvents(events []LogEvent, limit int) []LogEvent {
	if limit <= 0 || len(events) <= limit {
		return append([]LogEvent(nil), events...)
	}
	return append([]LogEvent(nil), events[len(events)-limit:]...)
}
