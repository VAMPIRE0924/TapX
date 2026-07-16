package pathmtu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type RouteCandidate struct {
	Destination netip.Addr
	Source      netip.Addr
	Device      string
	MTU         int
}

type ipRouteRow struct {
	Destination string `json:"dst"`
	Device      string `json:"dev"`
	Source      string `json:"prefsrc"`
	MTU         int    `json:"mtu"`
}

type ipLinkRow struct {
	IfName string `json:"ifname"`
	MTU    int    `json:"mtu"`
}

// DiscoverRouteCandidate reads Linux route state through iproute2 JSON. The
// result is only a probe candidate; it is not a peer-confirmed path MTU.
func DiscoverRouteCandidate(ctx context.Context, destination netip.Addr, runner CommandRunner) (RouteCandidate, error) {
	if !destination.IsValid() {
		return RouteCandidate{}, fmt.Errorf("destination is required")
	}
	if runner == nil {
		runner = runCommand
	}
	routeJSON, err := runner(ctx, "ip", "-json", "route", "get", destination.String())
	if err != nil {
		return RouteCandidate{}, fmt.Errorf("pathmtu: inspect route to %s: %w", destination, err)
	}
	candidate, err := parseRouteCandidate(routeJSON, destination)
	if err != nil {
		return RouteCandidate{}, err
	}
	if candidate.MTU > 0 {
		return candidate, nil
	}
	linkJSON, err := runner(ctx, "ip", "-json", "link", "show", "dev", candidate.Device)
	if err != nil {
		return RouteCandidate{}, fmt.Errorf("pathmtu: inspect link %s: %w", candidate.Device, err)
	}
	mtu, err := parseLinkMTU(linkJSON, candidate.Device)
	if err != nil {
		return RouteCandidate{}, err
	}
	candidate.MTU = mtu
	return candidate, nil
}

func parseRouteCandidate(data []byte, destination netip.Addr) (RouteCandidate, error) {
	var rows []ipRouteRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return RouteCandidate{}, fmt.Errorf("pathmtu: decode route JSON: %w", err)
	}
	if len(rows) == 0 {
		return RouteCandidate{}, fmt.Errorf("pathmtu: no route to %s", destination)
	}
	row := rows[0]
	if strings.TrimSpace(row.Device) == "" {
		return RouteCandidate{}, fmt.Errorf("pathmtu: route to %s has no device", destination)
	}
	candidate := RouteCandidate{Destination: destination, Device: row.Device, MTU: row.MTU}
	if row.Source != "" {
		source, err := netip.ParseAddr(row.Source)
		if err != nil {
			return RouteCandidate{}, fmt.Errorf("pathmtu: invalid route source %q: %w", row.Source, err)
		}
		candidate.Source = source.Unmap()
	}
	return candidate, nil
}

func parseLinkMTU(data []byte, device string) (int, error) {
	var rows []ipLinkRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, fmt.Errorf("pathmtu: decode link JSON: %w", err)
	}
	if len(rows) == 0 || rows[0].MTU <= 0 {
		return 0, fmt.Errorf("pathmtu: link %s has no usable MTU", device)
	}
	return rows[0].MTU, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
