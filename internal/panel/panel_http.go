package panel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) panelHTTPClient(ctx context.Context) (*http.Client, error) {
	cfg, err := s.store.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	outbound := "direct"
	for _, settings := range cfg.Settings {
		if settings.Enabled {
			if value := strings.TrimSpace(settings.PanelOutbound); value != "" {
				outbound = value
			}
			break
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if outbound == "direct" {
		return &http.Client{Transport: transport}, nil
	}
	transport.Proxy = nil
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		host, portText, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("panel outbound split target %q: %w", address, err)
		}
		port, err := strconv.ParseUint(portText, 10, 16)
		if err != nil || port == 0 {
			return nil, fmt.Errorf("panel outbound target port %q is invalid", portText)
		}
		return s.runtime.DialXrayTCP(dialCtx, outbound, host, uint16(port))
	}
	return &http.Client{Transport: transport}, nil
}
