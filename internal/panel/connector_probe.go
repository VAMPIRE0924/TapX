package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
)

type connectorTestRequest struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
}

type connectorTestResult struct {
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	Target              string `json:"target"`
	Network             string `json:"network"`
	DelayMS             int64  `json:"delayMs,omitempty"`
	Confirmed           bool   `json:"confirmed"`
	Active              bool   `json:"active"`
	Message             string `json:"message"`
	DeviceName          string `json:"deviceName,omitempty"`
	ConfirmedPathMTU    int    `json:"confirmedPathMtu,omitempty"`
	EffectiveNetworkMTU int    `json:"effectiveNetworkMtu,omitempty"`
	MaxDatagramPayload  int    `json:"maxDatagramPayload,omitempty"`
	TCPMSSIPv4          int    `json:"tcpMssIpv4,omitempty"`
	TCPMSSIPv6          int    `json:"tcpMssIpv6,omitempty"`
	UploadBytes         uint64 `json:"uploadBytes,omitempty"`
	DownloadBytes       uint64 `json:"downloadBytes,omitempty"`
	UploadBPS           uint64 `json:"uploadBps,omitempty"`
	DownloadBPS         uint64 `json:"downloadBps,omitempty"`
	DurationMS          int64  `json:"durationMs,omitempty"`
}

func (s *Server) handleConnectorTest(w http.ResponseWriter, r *http.Request) {
	var request connectorTestRequest
	body, err := readBody(r)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("connector id is required"))
		return
	}
	if request.Kind != "channel" && request.Kind != "path-mtu" && request.Kind != "throughput" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("kind must be channel, path-mtu, or throughput"))
		return
	}
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	connector, ok := findConnector(cfg.Connectors, request.ID)
	if !ok {
		writeErrorStatus(w, http.StatusNotFound, fmt.Errorf("connector %s not found", request.ID))
		return
	}
	duration := time.Duration(request.DurationSeconds * float64(time.Second))
	if duration <= 0 {
		duration = 2 * time.Second
	}
	if duration > 10*time.Second {
		duration = 10 * time.Second
	}
	timeout := 8 * time.Second
	if request.Kind == "throughput" {
		timeout = 2*duration + 10*time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result, err := s.probeConnector(ctx, cfg, connector, request.Kind, duration)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) probeConnector(ctx context.Context, cfg config.RuntimeConfig, connector model.Connector, kind string, duration time.Duration) (connectorTestResult, error) {
	pipe, running := connectorRuntimePipe(s.runtime.State(), connector.ID)
	result := connectorTestResult{
		ID: connector.ID, Kind: kind, Network: connectorProbeNetwork(cfg, connector),
		Target: net.JoinHostPort(connector.Remote, strconv.Itoa(int(connector.Port))),
	}
	if running {
		result.DeviceName = pipe.DeviceName
		result.ConfirmedPathMTU = pipe.ConfirmedPathMTU
		result.EffectiveNetworkMTU = pipe.EffectiveNetworkMTU
		result.MaxDatagramPayload = pipe.MaxDatagramPayload
		result.TCPMSSIPv4 = pipe.TCPMSSIPv4
		result.TCPMSSIPv6 = pipe.TCPMSSIPv6
		if pipe.RemoteAddr != "" {
			result.Target = pipe.RemoteAddr
		}
		if pipe.LastError != "" {
			return result, fmt.Errorf("connector runtime failed: %s", pipe.LastError)
		}
	}

	if connector.Transport == model.TransportTCP || connector.Transport == model.TransportXray || connector.Transport == model.TransportUDP && kind != "path-mtu" {
		diagnostic, err := s.runtime.DiagnoseConnector(ctx, connector.ID, kind, duration)
		if err != nil {
			return result, err
		}
		result.Active = true
		result.Confirmed = true
		result.Target = diagnostic.Target
		result.Network = diagnostic.Transport
		result.DelayMS = durationMilliseconds(diagnostic.Delay)
		if kind == "path-mtu" {
			if diagnostic.PathMTU > 0 {
				result.ConfirmedPathMTU = diagnostic.PathMTU
				result.EffectiveNetworkMTU = diagnostic.PathMTU
				result.Message = "A full device-sized frame was integrity-checked through the Xray stream; outer segmentation remains managed by Xray and kernel PMTUD."
			} else {
				result.TCPMSSIPv4 = diagnostic.TCPMSS
				result.TCPMSSIPv6 = diagnostic.TCPMSS
				result.Message = "Kernel TCP MSS was read from a fresh diagnostic stream over the configured connector."
			}
		} else if kind == "throughput" {
			result.UploadBytes = diagnostic.UploadBytes
			result.DownloadBytes = diagnostic.DownloadBytes
			result.UploadBPS = diagnostic.UploadBPS
			result.DownloadBPS = diagnostic.DownloadBPS
			result.DurationMS = diagnostic.Duration.Milliseconds()
			result.Message = "Bidirectional payload throughput was measured over an isolated TapX diagnostic stream."
		} else {
			result.Message = "The remote TapX listener acknowledged the diagnostic control stream."
		}
		return result, nil
	}

	if !running {
		return result, fmt.Errorf("connector %s has no active runtime pipe", connector.ID)
	}
	switch kind {
	case "channel":
		if pipe.ConfirmedPathMTU > 0 {
			result.Confirmed = true
			result.Message = "The UDP peer completed TapX path negotiation on this outer transport; no inner interface address was used."
			return result, nil
		}
		return result, fmt.Errorf("this transport has no active in-band acknowledgement yet")
	case "path-mtu":
		if pipe.ConfirmedPathMTU <= 0 {
			return result, fmt.Errorf("this connector has no peer-confirmed path MTU; enable automatic link optimization and apply the runtime")
		}
		result.Confirmed = true
		result.Active = true
		result.Message = "The value was peer-confirmed by TapX over the outer UDP path and does not depend on a TUN/TAP address."
		return result, nil
	case "throughput":
		return result, fmt.Errorf("active throughput control is not available for %s yet", result.Network)
	default:
		return result, fmt.Errorf("unsupported connector diagnostic %q", kind)
	}
}

func connectorRuntimePipe(state RuntimeState, connectorID string) (RuntimePipeState, bool) {
	for _, pipes := range [][]RuntimePipeState{state.UDPPipes, state.TCPPipes, state.XrayPipes} {
		for _, pipe := range pipes {
			if pipe.EndpointKind == "connector" && pipe.EndpointID == connectorID {
				return pipe, true
			}
		}
	}
	return RuntimePipeState{}, false
}

func connectorProbeNetwork(cfg config.RuntimeConfig, connector model.Connector) string {
	if connector.Transport == model.TransportUDP {
		if connector.RawUDP.DTLS.Enabled {
			return "dtls"
		}
		return "udp"
	}
	if connector.Transport == model.TransportTCP {
		if connector.RawTCP.TLS.Enabled {
			return "tls"
		}
		return "tcp"
	}
	for _, profile := range cfg.XrayProfiles {
		if profile.ID == connector.XrayProfileID {
			return "xray/" + strings.ToLower(strings.TrimSpace(profile.Network))
		}
	}
	return "xray"
}

func durationMilliseconds(duration time.Duration) int64 {
	delay := duration.Milliseconds()
	if delay < 1 {
		return 1
	}
	return delay
}

func findConnector(connectors []model.Connector, id string) (model.Connector, bool) {
	for _, connector := range connectors {
		if connector.ID == id {
			return connector, true
		}
	}
	return model.Connector{}, false
}
