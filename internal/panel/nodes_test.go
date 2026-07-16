package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestManagedNodeRegistryAndRemoteControlPlane(t *testing.T) {
	var mu sync.Mutex
	remoteConfig := map[string]any{"Devices": []any{}}
	applyCount := 0
	updateCount := 0
	connectorTestCount := 0
	trafficResetPaths := []string{}
	integrationRequests := []string{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer node-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/diagnostics":
			writeJSON(w, http.StatusOK, map[string]any{
				"product": "TapX", "version": "v0.2.0",
				"components":   map[string]any{"panel": "v0.2.0", "tapx": "v0.2.0", "embeddedXray": "v26.7.11"},
				"process":      map[string]any{"uptimeSecond": 90061},
				"objectCounts": map[string]any{"devices": 1, "listeners": 2, "clients": 3, "connectors": 4, "routes": 5},
			})
		case "/api/dashboard":
			writeJSON(w, http.StatusOK, map[string]any{"system": map[string]any{
				"cpuPercent": 12.5, "memoryUsed": 25, "memoryTotal": 100,
			}})
		case "/api/stats":
			writeJSON(w, http.StatusOK, map[string]any{"byEndpoint": []any{
				map[string]any{"id": "connector:connector-a", "name": "connector-a", "kind": "connector"},
			}})
		case "/api/system/interfaces":
			writeJSON(w, http.StatusOK, map[string]any{"interfaces": []string{"eth0", "ens3"}})
		case "/api/share/clients/client-a":
			writeJSON(w, http.StatusOK, map[string]any{"share": map[string]any{"link": "raw://remote-client"}})
		case "/api/connectors/test":
			var request connectorTestRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			connectorTestCount++
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{
				"id": request.ID, "kind": request.Kind, "target": "remote", "network": "udp", "confirmed": true,
			}})
		case "/api/connectors/connector-a/traffic/reset", "/api/listeners/listener-a/traffic/reset", "/api/clients/client-a/traffic/reset":
			mu.Lock()
			trafficResetPaths = append(trafficResetPaths, r.URL.Path)
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": map[string]any{
				"Connectors": []any{map[string]any{"ID": "connector-a", "TrafficResetAt": 10}},
				"Listeners":  []any{map[string]any{"ID": "listener-a", "TrafficResetAt": 11}},
				"Clients":    []any{map[string]any{"ID": "client-a", "TrafficResetAt": 12}},
			}})
		case "/api/integrations/warp/data":
			if r.Method != http.MethodGet {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			mu.Lock()
			integrationRequests = append(integrationRequests, r.Method+" "+r.URL.RequestURI())
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"device_id": "remote-warp"}})
		case "/api/integrations/nord/servers":
			if r.Method != http.MethodGet || r.URL.Query().Get("countryId") != "228" {
				http.Error(w, "request", http.StatusBadRequest)
				return
			}
			mu.Lock()
			integrationRequests = append(integrationRequests, r.Method+" "+r.URL.RequestURI())
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"servers": []any{}}})
		case "/api/integrations/nord/login":
			var request map[string]any
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&request) != nil || request["token"] != "remote-token" {
				http.Error(w, "request", http.StatusBadRequest)
				return
			}
			mu.Lock()
			integrationRequests = append(integrationRequests, r.Method+" "+r.URL.RequestURI())
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"private_key": "remote-key"}})
		case "/api/config":
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodPut {
				if err := json.NewDecoder(r.Body).Decode(&remoteConfig); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"config": remoteConfig})
		case "/api/runtime/apply":
			mu.Lock()
			applyCount++
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		case "/api/updates/panel":
			if r.Method == http.MethodGet {
				writeJSON(w, http.StatusOK, updateCatalog{Versions: []updateVersion{{Version: "0.3.0", Latest: true, Installable: true}}})
				return
			}
			mu.Lock()
			updateCount++
			mu.Unlock()
			writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "restarting": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)

	parsed, err := url.Parse(remote.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, found := strings.Cut(parsed.Host, ":")
	if !found {
		t.Fatalf("remote URL host %q has no port", parsed.Host)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)
	nodeBody := mustJSON(t, map[string]any{
		"ID": "node-lab", "Enabled": true, "Name": "lab", "Protocol": "http",
		"Host": host, "Port": port, "BasePath": "/", "AllowPrivateAddress": true,
		"TLSVerify": "system", "APIToken": "node-secret",
	})
	draft := postJSON(t, server.URL+"/api/nodes/test", nodeBody, http.StatusOK)["node"].(map[string]any)
	if draft["Status"] != "online" {
		t.Fatalf("draft node test = %+v", draft)
	}
	if nodes := getJSON(t, server.URL+"/api/nodes", http.StatusOK)["nodes"].([]any); len(nodes) != 0 {
		t.Fatalf("draft node test persisted registry: %+v", nodes)
	}
	putJSON(t, server.URL+"/api/nodes/node-lab", nodeBody, http.StatusOK)
	putJSON(t, server.URL+"/api/nodes/node-lab", mustJSON(t, map[string]any{
		"ID": "node-lab", "Enabled": true, "Name": "lab-edited", "Protocol": "http",
		"Host": host, "Port": port, "BasePath": "/", "AllowPrivateAddress": true,
		"TLSVerify": "system", "APIToken": "",
	}), http.StatusOK)

	listed := getJSON(t, server.URL+"/api/nodes", http.StatusOK)
	nodes := listed["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("nodes = %+v, want one", nodes)
	}
	view := nodes[0].(map[string]any)
	if _, leaked := view["APIToken"]; leaked {
		t.Fatalf("node list leaked API token: %+v", view)
	}
	if view["APITokenConfigured"] != true {
		t.Fatalf("node token configured = %+v, want true", view["APITokenConfigured"])
	}

	tested := postJSON(t, server.URL+"/api/nodes/node-lab/test", nil, http.StatusOK)["node"].(map[string]any)
	if tested["Status"] != "online" || tested["PanelVersion"] != "v0.2.0" || tested["Uptime"] != "1d 1h" {
		t.Fatalf("tested node = %+v", tested)
	}
	if tested["CPU"].(float64) != 12.5 || tested["Memory"].(float64) != 25 {
		t.Fatalf("tested node resource data = %+v", tested)
	}

	putJSON(t, server.URL+"/api/nodes/node-lab/config", []byte(`{"Devices":[],"Settings":[]}`), http.StatusOK)
	proxied := getJSON(t, server.URL+"/api/nodes/node-lab/config", http.StatusOK)
	configValue := proxied["config"].(map[string]any)
	if _, ok := configValue["Devices"].([]any); !ok {
		t.Fatalf("proxied config = %+v", proxied)
	}
	postJSON(t, server.URL+"/api/nodes/node-lab/runtime/apply", nil, http.StatusOK)
	postJSON(t, server.URL+"/api/nodes/node-lab/update", nil, http.StatusAccepted)
	stats := getJSON(t, server.URL+"/api/nodes/node-lab/stats", http.StatusOK)
	if len(stats["byEndpoint"].([]any)) != 1 {
		t.Fatalf("proxied node stats = %+v", stats)
	}
	interfaces := getJSON(t, server.URL+"/api/nodes/node-lab/system/interfaces", http.StatusOK)
	if len(interfaces["interfaces"].([]any)) != 2 {
		t.Fatalf("proxied node interfaces = %+v", interfaces)
	}
	share := getJSON(t, server.URL+"/api/nodes/node-lab/share/clients/client-a", http.StatusOK)
	if share["share"].(map[string]any)["link"] != "raw://remote-client" {
		t.Fatalf("proxied client share = %+v", share)
	}
	postJSON(t, server.URL+"/api/nodes/node-lab/connectors/test", mustJSON(t, map[string]any{
		"id": "connector-a", "kind": "channel",
	}), http.StatusOK)
	for _, path := range []string{
		"/api/nodes/node-lab/connectors/connector-a/traffic/reset",
		"/api/nodes/node-lab/listeners/listener-a/traffic/reset",
		"/api/nodes/node-lab/clients/client-a/traffic/reset",
	} {
		postJSON(t, server.URL+path, nil, http.StatusOK)
	}
	warp := getJSON(t, server.URL+"/api/nodes/node-lab/integrations/warp/data", http.StatusOK)
	if warp["data"].(map[string]any)["device_id"] != "remote-warp" {
		t.Fatalf("proxied WARP data = %+v", warp)
	}
	getJSON(t, server.URL+"/api/nodes/node-lab/integrations/nord/servers?countryId=228", http.StatusOK)
	postJSON(t, server.URL+"/api/nodes/node-lab/integrations/nord/login", mustJSON(t, map[string]any{"token": "remote-token"}), http.StatusOK)
	mu.Lock()
	if applyCount != 1 {
		t.Fatalf("remote apply count = %d, want 1", applyCount)
	}
	if updateCount != 1 {
		t.Fatalf("remote update count = %d, want 1", updateCount)
	}
	if connectorTestCount != 1 {
		t.Fatalf("remote connector test count = %d, want 1", connectorTestCount)
	}
	if len(trafficResetPaths) != 3 {
		t.Fatalf("remote traffic reset paths = %v, want three", trafficResetPaths)
	}
	if len(integrationRequests) != 3 {
		t.Fatalf("remote integration requests = %v, want three", integrationRequests)
	}
	mu.Unlock()

	deleteJSON(t, server.URL+"/api/nodes/node-lab", http.StatusOK)
	if remaining := getJSON(t, server.URL+"/api/nodes", http.StatusOK)["nodes"].([]any); len(remaining) != 0 {
		t.Fatalf("remaining nodes = %+v, want none", remaining)
	}
}

func TestResolveManagedNodeIPsNeverAllowsInvalidDestinations(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "224.0.0.1", "ff02::1"} {
		if ips, err := resolveManagedNodeIPs(t.Context(), host, true); err == nil {
			t.Fatalf("resolveManagedNodeIPs(%q) = %v, want rejection", host, ips)
		}
	}
	if ips, err := resolveManagedNodeIPs(t.Context(), "127.0.0.1", true); err != nil || len(ips) != 1 {
		t.Fatalf("explicit private node address = %v, %v", ips, err)
	}
}

func TestManagedNodeRejectsImplicitPrivateAccess(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	putJSON(t, server.URL+"/api/nodes/private", mustJSON(t, map[string]any{
		"ID": "private", "Enabled": true, "Name": "private", "Protocol": "https",
		"Host": "127.0.0.1", "Port": 443, "BasePath": "/", "AllowPrivateAddress": false,
		"TLSVerify": "skip", "APIToken": "secret",
	}), http.StatusOK)
	postJSON(t, server.URL+"/api/nodes/private/test", nil, http.StatusBadGateway)

	putJSON(t, server.URL+"/api/nodes/plain", mustJSON(t, map[string]any{
		"ID": "plain", "Enabled": true, "Name": "plain", "Protocol": "http",
		"Host": "panel.example.com", "Port": 80, "BasePath": "/", "AllowPrivateAddress": false,
		"TLSVerify": "system", "APIToken": "secret",
	}), http.StatusBadRequest)
}

func TestManagedNodeRegistryIsIncludedInPortableState(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)
	putJSON(t, server.URL+"/api/nodes/node-backup", mustJSON(t, map[string]any{
		"ID": "node-backup", "Enabled": true, "Name": "backup", "Protocol": "https",
		"Host": "panel.example.com", "Port": 443, "BasePath": "/tapx/",
		"AllowPrivateAddress": false, "TLSVerify": "system", "APIToken": "backup-secret",
	}), http.StatusOK)

	integrations, err := store.ListIntegrations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := integrations[managedNodesIntegrationName]
	if !ok || !strings.Contains(string(raw), "backup-secret") {
		t.Fatalf("managed node registry missing from portable state: %s", raw)
	}
}
