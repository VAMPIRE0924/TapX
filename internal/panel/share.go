package panel

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"tapx/internal/config"
	"tapx/internal/model"
)

type ClientShare struct {
	ClientID string         `json:"clientId"`
	Type     string         `json:"type"`
	Link     string         `json:"link"`
	QRPNG    string         `json:"qrPng"`
	Payload  SharePayload   `json:"payload"`
	Warnings []string       `json:"warnings,omitempty"`
	Objects  ShareObjectIDs `json:"objects"`
}

type ShareObjectIDs struct {
	ListenerID  string `json:"listenerId,omitempty"`
	ConnectorID string `json:"connectorId,omitempty"`
	RouteID     string `json:"routeId,omitempty"`
	DeviceID    string `json:"deviceId,omitempty"`
	AddressID   string `json:"addressId,omitempty"`
	VKeyID      string `json:"vkeyId,omitempty"`
}

type SharePayload struct {
	Version       int                   `json:"version"`
	Client        ShareClient           `json:"client"`
	Transport     model.Transport       `json:"transport,omitempty"`
	Listener      *ShareListener        `json:"listener,omitempty"`
	Connector     *ShareConnector       `json:"connector,omitempty"`
	Route         *model.Route          `json:"route,omitempty"`
	Device        *model.Device         `json:"device,omitempty"`
	AddressLimit  *model.AddressLimit   `json:"addressLimit,omitempty"`
	VKey          *ShareVKey            `json:"vkey,omitempty"`
	XrayProfile   *model.XrayProfile    `json:"xrayProfile,omitempty"`
	RawUDP        *model.RawUDPSettings `json:"rawUdp,omitempty"`
	RawTCP        *model.RawTCPSettings `json:"rawTcp,omitempty"`
	ResolvedRoute config.RuntimeBinding `json:"resolvedRoute"`
}

type ShareClient struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Email           string `json:"email,omitempty"`
	CredentialType  string `json:"credentialType,omitempty"`
	CredentialValue string `json:"credentialValue,omitempty"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	TrafficCap      uint64 `json:"trafficCap,omitempty"`
}

type ShareListener struct {
	ID            string               `json:"id"`
	Name          string               `json:"name,omitempty"`
	BindHost      string               `json:"bindHost,omitempty"`
	BindPort      uint16               `json:"bindPort,omitempty"`
	Transport     model.Transport      `json:"transport"`
	XrayProfileID string               `json:"xrayProfileId,omitempty"`
	RawUDP        model.RawUDPSettings `json:"rawUdp,omitempty"`
	RawTCP        model.RawTCPSettings `json:"rawTcp,omitempty"`
}

type ShareConnector struct {
	ID            string               `json:"id"`
	Name          string               `json:"name,omitempty"`
	Remote        string               `json:"remote,omitempty"`
	Port          uint16               `json:"port,omitempty"`
	Transport     model.Transport      `json:"transport"`
	XrayProfileID string               `json:"xrayProfileId,omitempty"`
	RawUDP        model.RawUDPSettings `json:"rawUdp,omitempty"`
	RawTCP        model.RawTCPSettings `json:"rawTcp,omitempty"`
}

type ShareVKey struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value"`
}

func BuildClientShare(cfg config.RuntimeConfig, clientID string) (ClientShare, error) {
	index := shareIndexFromConfig(cfg)
	client, ok := index.clients[clientID]
	if !ok {
		return ClientShare{}, ErrNotFound
	}
	if err := config.ValidateForSave(cfg); err != nil {
		return ClientShare{}, err
	}

	resolved, objects, warnings := index.resolveClient(client)
	payload := SharePayload{
		Version: 1,
		Client: ShareClient{
			ID:              client.ID,
			Name:            client.Name,
			Email:           client.Email,
			CredentialType:  client.CredentialType,
			CredentialValue: client.CredentialValue,
			ExpiresAt:       client.ExpiresAt,
			TrafficCap:      client.TrafficCap,
		},
		ResolvedRoute: resolved,
	}
	if objects.ListenerID != "" {
		if listener, ok := index.listeners[objects.ListenerID]; ok {
			payload.Transport = listener.Transport
			payload.Listener = shareListener(listener)
			payload.RawUDP = &listener.RawUDP
			payload.RawTCP = &listener.RawTCP
			if listener.XrayProfileID != "" {
				objects.VKeyID = ""
				if profile, ok := index.xray[listener.XrayProfileID]; ok {
					payload.XrayProfile = &profile
				}
			}
		}
	}
	if objects.ConnectorID != "" {
		if connector, ok := index.connectors[objects.ConnectorID]; ok {
			if payload.Transport == "" {
				payload.Transport = connector.Transport
			}
			payload.Connector = shareConnector(connector)
			if payload.RawUDP == nil {
				payload.RawUDP = &connector.RawUDP
			}
			if payload.RawTCP == nil {
				payload.RawTCP = &connector.RawTCP
			}
			if connector.XrayProfileID != "" && payload.XrayProfile == nil {
				if profile, ok := index.xray[connector.XrayProfileID]; ok {
					payload.XrayProfile = &profile
				}
			}
		}
	}
	if objects.RouteID != "" {
		if route, ok := index.routes[objects.RouteID]; ok {
			payload.Route = &route
		}
	}
	if objects.DeviceID != "" {
		if device, ok := index.devices[objects.DeviceID]; ok {
			payload.Device = &device
		}
	}
	if objects.AddressID != "" {
		if address, ok := index.addresses[objects.AddressID]; ok {
			payload.AddressLimit = &address
		}
	}
	if objects.VKeyID != "" {
		if vkey, ok := index.vkeys[objects.VKeyID]; ok && vkey.Enabled {
			payload.VKey = &ShareVKey{ID: vkey.ID, Name: vkey.Name, Value: vkey.Value}
		}
	}

	link, shareType, linkWarnings, err := buildShareLink(payload)
	if err != nil {
		return ClientShare{}, err
	}
	warnings = append(warnings, linkWarnings...)
	qr, err := qrDataURL(link)
	if err != nil {
		return ClientShare{}, err
	}
	return ClientShare{
		ClientID: client.ID,
		Type:     shareType,
		Link:     link,
		QRPNG:    qr,
		Payload:  payload,
		Warnings: warnings,
		Objects:  objects,
	}, nil
}

type shareIndex struct {
	devices    map[string]model.Device
	listeners  map[string]model.Listener
	connectors map[string]model.Connector
	clients    map[string]model.Client
	routes     map[string]model.Route
	vkeys      map[string]model.VKey
	addresses  map[string]model.AddressLimit
	xray       map[string]model.XrayProfile
}

func shareIndexFromConfig(cfg config.RuntimeConfig) shareIndex {
	out := shareIndex{
		devices:    map[string]model.Device{},
		listeners:  map[string]model.Listener{},
		connectors: map[string]model.Connector{},
		clients:    map[string]model.Client{},
		routes:     map[string]model.Route{},
		vkeys:      map[string]model.VKey{},
		addresses:  map[string]model.AddressLimit{},
		xray:       map[string]model.XrayProfile{},
	}
	for _, item := range cfg.Devices {
		out.devices[item.ID] = item
	}
	for _, item := range cfg.Listeners {
		out.listeners[item.ID] = item
	}
	for _, item := range cfg.Connectors {
		out.connectors[item.ID] = item
	}
	for _, item := range cfg.Clients {
		out.clients[item.ID] = item
	}
	for _, item := range cfg.Routes {
		out.routes[item.ID] = item
	}
	for _, item := range cfg.VKeys {
		out.vkeys[item.ID] = item
	}
	for _, item := range cfg.Addresses {
		out.addresses[item.ID] = item
	}
	for _, item := range cfg.XrayProfiles {
		out.xray[item.ID] = item
	}
	return out
}

func (idx shareIndex) resolveClient(client model.Client) (config.RuntimeBinding, ShareObjectIDs, []string) {
	warnings := []string{}
	objects := ShareObjectIDs{
		ListenerID: client.ListenerID,
		RouteID:    client.Binding.RouteID,
		DeviceID:   firstNonEmpty(client.Binding.DeviceID),
		AddressID:  firstNonEmpty(client.AddressID, client.Binding.AddressID),
		VKeyID:     client.Binding.VKeyID,
	}
	resolved := config.RuntimeBinding{
		VKeyValue:   idx.vkeyValue(client.Binding.VKeyID),
		ClientID:    client.ID,
		RouteID:     client.Binding.RouteID,
		DeviceID:    client.Binding.DeviceID,
		ConnectorID: client.Binding.ConnectorID,
		AddressID:   firstNonEmpty(client.AddressID, client.Binding.AddressID),
	}
	if client.Binding.ConnectorID != "" {
		objects.ConnectorID = client.Binding.ConnectorID
	}
	if client.Binding.RouteID != "" {
		if route, ok := idx.routes[client.Binding.RouteID]; ok {
			objects.ListenerID = firstNonEmpty(objects.ListenerID, route.ListenerID)
			objects.ConnectorID = firstNonEmpty(objects.ConnectorID, route.ConnectorID)
			objects.DeviceID = firstNonEmpty(objects.DeviceID, route.DeviceID)
			objects.AddressID = firstNonEmpty(objects.AddressID, route.AddressID)
			objects.VKeyID = firstNonEmpty(objects.VKeyID, route.VKeyID)
			resolved.VKeyValue = firstNonEmpty(resolved.VKeyValue, idx.vkeyValue(route.VKeyID))
			resolved.DeviceID = firstNonEmpty(resolved.DeviceID, route.DeviceID)
			resolved.ConnectorID = firstNonEmpty(resolved.ConnectorID, route.ConnectorID)
			resolved.AddressID = firstNonEmpty(resolved.AddressID, route.AddressID)
		}
	}
	if objects.AddressID != "" {
		if address, ok := idx.addresses[objects.AddressID]; ok {
			objects.DeviceID = firstNonEmpty(objects.DeviceID, address.DeviceID)
		}
	}
	if objects.ListenerID == "" && objects.ConnectorID == "" {
		warnings = append(warnings, "client is not bound to a listener or connector")
	}
	return resolved, objects, warnings
}

func (idx shareIndex) vkeyValue(id string) string {
	if id == "" {
		return ""
	}
	item, ok := idx.vkeys[id]
	if !ok || !item.Enabled {
		return ""
	}
	return item.Value
}

func shareListener(listener model.Listener) *ShareListener {
	return &ShareListener{
		ID:            listener.ID,
		Name:          listener.Name,
		BindHost:      listener.BindHost,
		BindPort:      listener.BindPort,
		Transport:     listener.Transport,
		XrayProfileID: listener.XrayProfileID,
		RawUDP:        listener.RawUDP,
		RawTCP:        listener.RawTCP,
	}
}

func shareConnector(connector model.Connector) *ShareConnector {
	return &ShareConnector{
		ID:            connector.ID,
		Name:          connector.Name,
		Remote:        connector.Remote,
		Port:          connector.Port,
		Transport:     connector.Transport,
		XrayProfileID: connector.XrayProfileID,
		RawUDP:        connector.RawUDP,
		RawTCP:        connector.RawTCP,
	}
}

func buildShareLink(payload SharePayload) (string, string, []string, error) {
	if payload.Transport == model.TransportXray && payload.XrayProfile != nil {
		if strings.EqualFold(payload.XrayProfile.InboundProtocol, "vless") {
			link, warnings, err := buildVLESSShareLink(payload)
			return link, "xray", warnings, err
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", "", nil, err
	}
	compressed, err := gzipPayload(raw)
	if err != nil {
		return "", "", nil, err
	}
	link := "tapx://client/gzip/" + base64.RawURLEncoding.EncodeToString(compressed)
	shareType := "tapx"
	if payload.Transport == model.TransportXray {
		shareType = "xray"
	}
	return link, shareType, nil, nil
}

func buildVLESSShareLink(payload SharePayload) (string, []string, error) {
	if payload.Listener == nil {
		return "", nil, fmt.Errorf("xray share requires a listener")
	}
	if strings.TrimSpace(payload.Client.CredentialValue) == "" {
		return "", nil, fmt.Errorf("xray share requires Client.CredentialValue")
	}
	host := normalizeShareHost(payload.Listener.BindHost)
	warnings := []string{}
	if host == "" {
		host = "server.example"
		warnings = append(warnings, "listener bind host is wildcard; replace server.example in the share link")
	}
	values := url.Values{}
	values.Set("type", firstNonEmpty(payload.XrayProfile.Network, "tcp"))
	values.Set("security", firstNonEmpty(payload.XrayProfile.Security, "none"))
	name := firstNonEmpty(payload.Client.Name, payload.Client.Email, payload.Client.ID)
	u := url.URL{
		Scheme:   "vless",
		User:     url.User(payload.Client.CredentialValue),
		Host:     net.JoinHostPort(host, strconv.Itoa(int(payload.Listener.BindPort))),
		RawQuery: values.Encode(),
		Fragment: name,
	}
	return u.String(), warnings, nil
}

func normalizeShareHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return ""
	}
	return strings.Trim(host, "[]")
}

func qrDataURL(link string) (string, error) {
	png, err := qrcode.Encode(link, qrcode.Low, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

func gzipPayload(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
