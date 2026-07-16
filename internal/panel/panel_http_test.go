package panel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

type panelOutboundDialController struct {
	fakeRuntimeController
	lastTag string
}

func (c *panelOutboundDialController) DialXrayTCP(ctx context.Context, tag, host string, port uint16) (net.Conn, error) {
	c.lastTag = tag
	return (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
}

func TestPanelHTTPClientUsesConfiguredEmbeddedXrayConnector(t *testing.T) {
	store := newTestStore(t)
	cfg := config.RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{ID: "profile-a", Enabled: true, Runtime: model.XrayEmbedded}},
		Connectors:   []model.Connector{{ID: "connector-a", Enabled: true, Transport: model.TransportXray, XrayProfileID: "profile-a"}},
		Settings:     []model.Settings{{ID: "global", Enabled: true, PanelOutbound: "connector-a"}},
	}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	controller := &panelOutboundDialController{}
	runtime := NewRuntimeManager()
	runtime.controller = controller
	server := NewServer(store, runtime)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(target.Close)

	client, err := server.panelHTTPClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if controller.lastTag != "connector-a" {
		t.Fatalf("dial outbound tag = %q, want connector-a", controller.lastTag)
	}
}

func TestPanelHTTPClientDirectDoesNotRequireRuntime(t *testing.T) {
	store := newTestStore(t)
	if err := store.ReplaceConfig(context.Background(), config.RuntimeConfig{
		Settings: []model.Settings{{ID: "global", Enabled: true, PanelOutbound: "direct"}},
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(store)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)
	client, err := server.panelHTTPClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}
