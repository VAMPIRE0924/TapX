package panel

import (
	"net/http"
	"sync"
	"time"
)

const dashboardRecentLogLimit = 8

type DashboardReport struct {
	GeneratedAt  string             `json:"generatedAt"`
	Runtime      RuntimeState       `json:"runtime"`
	Stats        StatsReport        `json:"stats"`
	Rates        DashboardRates     `json:"rates"`
	Process      ProcessDiagnostic  `json:"process"`
	Fastpath     FastpathDiagnostic `json:"fastpath"`
	OpenWrt      OpenWrtDiagnostic  `json:"openwrt"`
	ObjectCounts map[string]int     `json:"objectCounts"`
	RecentLogs   []LogEvent         `json:"recentLogs"`
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
	report := DashboardReport{
		GeneratedAt:  now.Format(time.RFC3339Nano),
		Runtime:      state,
		Stats:        stats,
		Rates:        s.dashboard.Rates(stats.Totals, now),
		Process:      diag.Process,
		Fastpath:     diag.Fastpath,
		OpenWrt:      diag.OpenWrt,
		ObjectCounts: diag.ObjectCounts,
		RecentLogs:   recentLogEvents(s.logs.List(), dashboardRecentLogLimit),
	}
	writeJSON(w, http.StatusOK, report)
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
