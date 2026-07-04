package panel

import (
	"net/http"
	goruntime "runtime"
	"time"

	"tapx/internal/buildinfo"
	"tapx/internal/config"
	"tapx/internal/fastpath"
)

type DiagnosticReport struct {
	Product      string             `json:"product"`
	Version      string             `json:"version"`
	GeneratedAt  string             `json:"generatedAt"`
	Process      ProcessDiagnostic  `json:"process"`
	Fastpath     FastpathDiagnostic `json:"fastpath"`
	OpenWrt      OpenWrtDiagnostic  `json:"openwrt"`
	ObjectCounts map[string]int     `json:"objectCounts"`
	Runtime      RuntimeState       `json:"runtime"`
}

type ProcessDiagnostic struct {
	StartedAt    string `json:"startedAt"`
	UptimeSecond int64  `json:"uptimeSecond"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	GoVersion    string `json:"goVersion"`
	Goroutines   int    `json:"goroutines"`
	HeapAlloc    uint64 `json:"heapAlloc"`
	HeapSys      uint64 `json:"heapSys"`
}

type FastpathDiagnostic struct {
	ABI uint32 `json:"abi"`
}

type OpenWrtDiagnostic struct {
	CurrentBuildTarget string `json:"currentBuildTarget"`
	ExtraTargets       string `json:"extraTargets"`
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	now := time.Now().UTC()
	writeJSON(w, http.StatusOK, s.buildDiagnosticReport(cfg, now))
}

func (s *Server) buildDiagnosticReport(cfg config.RuntimeConfig, now time.Time) DiagnosticReport {
	var mem goruntime.MemStats
	goruntime.ReadMemStats(&mem)
	return DiagnosticReport{
		Product:     "TapX",
		Version:     buildinfo.Version,
		GeneratedAt: now.Format(time.RFC3339Nano),
		Process: ProcessDiagnostic{
			StartedAt:    s.started.UTC().Format(time.RFC3339Nano),
			UptimeSecond: int64(time.Since(s.started).Seconds()),
			GOOS:         goruntime.GOOS,
			GOARCH:       goruntime.GOARCH,
			GoVersion:    goruntime.Version(),
			Goroutines:   goruntime.NumGoroutine(),
			HeapAlloc:    mem.HeapAlloc,
			HeapSys:      mem.HeapSys,
		},
		Fastpath: FastpathDiagnostic{
			ABI: fastpath.ABI(),
		},
		OpenWrt: OpenWrtDiagnostic{
			CurrentBuildTarget: "x86-64",
			ExtraTargets:       "deferred",
		},
		ObjectCounts: objectCounts(cfg),
		Runtime:      s.runtime.State(),
	}
}

func objectCounts(cfg config.RuntimeConfig) map[string]int {
	return map[string]int{
		KindDevices:    len(cfg.Devices),
		KindListeners:  len(cfg.Listeners),
		KindConnectors: len(cfg.Connectors),
		KindClients:    len(cfg.Clients),
		KindRoutes:     len(cfg.Routes),
		KindVKeys:      len(cfg.VKeys),
		KindAddresses:  len(cfg.Addresses),
		KindXray:       len(cfg.XrayProfiles),
		KindSettings:   len(cfg.Settings),
	}
}
