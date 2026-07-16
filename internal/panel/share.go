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

	"tapx/internal/config"
	"tapx/internal/model"
)

type ClientShare struct {
	ClientID string         `json:"clientId"`
	Type     string         `json:"type"`
	Link     string         `json:"link"`
	Links    []string       `json:"links,omitempty"`
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
	ID                string `json:"id"`
	Name              string `json:"name,omitempty"`
	Email             string `json:"email,omitempty"`
	CredentialType    string `json:"credentialType,omitempty"`
	CredentialValue   string `json:"credentialValue,omitempty"`
	UUID              string `json:"uuid,omitempty"`
	Password          string `json:"password,omitempty"`
	Auth              string `json:"auth,omitempty"`
	ExpiresAt         int64  `json:"expiresAt,omitempty"`
	TrafficCap        uint64 `json:"trafficCap,omitempty"`
	UploadRateLimit   uint64 `json:"uploadRateLimit,omitempty"`
	DownloadRateLimit uint64 `json:"downloadRateLimit,omitempty"`
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

	resolved, objects, warnings := index.resolveClient(client)
	payload := SharePayload{
		Version: 1,
		Client: ShareClient{
			ID:                client.ID,
			Name:              client.Name,
			Email:             client.Email,
			CredentialType:    client.CredentialType,
			CredentialValue:   client.CredentialValue,
			UUID:              client.UUID,
			Password:          client.Password,
			Auth:              client.Auth,
			ExpiresAt:         client.ExpiresAt,
			TrafficCap:        client.TrafficCap,
			UploadRateLimit:   client.UploadRateLimit,
			DownloadRateLimit: client.DownloadRateLimit,
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

	links, shareType, linkWarnings, err := index.buildClientLinks(client, payload, objects)
	if err != nil {
		return ClientShare{}, err
	}
	warnings = append(warnings, linkWarnings...)
	return ClientShare{
		ClientID: client.ID,
		Type:     shareType,
		Link:     links[0],
		Links:    links,
		Payload:  payload,
		Warnings: warnings,
		Objects:  objects,
	}, nil
}

func (idx shareIndex) buildClientLinks(client model.Client, base SharePayload, objects ShareObjectIDs) ([]string, string, []string, error) {
	listenerIDs := uniqueStrings(append([]string{client.ListenerID}, append(client.ListenerIDs, objects.ListenerID)...))
	if len(listenerIDs) == 0 {
		link, shareType, warnings, err := buildShareLink(base)
		return []string{link}, shareType, warnings, err
	}

	links := make([]string, 0, len(listenerIDs))
	warnings := []string{}
	shareType := "tapx"
	for _, listenerID := range listenerIDs {
		listener, ok := idx.listeners[listenerID]
		if !ok {
			warnings = append(warnings, "listener "+listenerID+" was not found")
			continue
		}
		payload := base
		payload.Transport = listener.Transport
		payload.Listener = shareListener(listener)
		payload.RawUDP = &listener.RawUDP
		payload.RawTCP = &listener.RawTCP
		payload.XrayProfile = nil
		if listener.XrayProfileID != "" {
			if profile, found := idx.xray[listener.XrayProfileID]; found {
				payload.XrayProfile = &profile
			}
		}
		link, currentType, currentWarnings, err := buildShareLink(payload)
		if err != nil {
			return nil, "", warnings, fmt.Errorf("listener %s: %w", listenerID, err)
		}
		if len(links) == 0 {
			shareType = currentType
		}
		links = append(links, link)
		warnings = append(warnings, currentWarnings...)
	}
	if len(links) == 0 {
		return nil, "", warnings, fmt.Errorf("client has no available listener")
	}
	return uniqueStrings(links), shareType, warnings, nil
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
		VKeyValue:         idx.vkeyValue(client.Binding.VKeyID),
		ClientID:          client.ID,
		RouteID:           client.Binding.RouteID,
		DeviceID:          client.Binding.DeviceID,
		ConnectorID:       client.Binding.ConnectorID,
		AddressID:         firstNonEmpty(client.AddressID, client.Binding.AddressID),
		UploadRateLimit:   client.UploadRateLimit,
		DownloadRateLimit: client.DownloadRateLimit,
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
	shareHost := listener.BindHost
	if listener.ShareAddressStrategy == "custom" && strings.TrimSpace(listener.ShareAddress) != "" {
		shareHost = listener.ShareAddress
	}
	return &ShareListener{
		ID:            listener.ID,
		Name:          listener.Name,
		BindHost:      shareHost,
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
	if payload.Transport == model.TransportTCP || payload.Transport == model.TransportUDP {
		link, warnings, err := buildRawShareLink(payload)
		return link, "tapx", warnings, err
	}
	if payload.Transport == model.TransportXray && payload.XrayProfile != nil {
		switch strings.ToLower(payload.XrayProfile.InboundProtocol) {
		case "vless", "trojan":
			link, warnings, err := buildURLXrayShareLink(payload)
			return link, "xray", warnings, err
		case "vmess":
			link, warnings, err := buildVMessShareLink(payload)
			return link, "xray", warnings, err
		case "shadowsocks":
			link, warnings, err := buildShadowsocksShareLink(payload)
			return link, "xray", warnings, err
		case "hysteria", "hysteria2":
			link, warnings, err := buildHysteriaShareLink(payload)
			return link, "xray", warnings, err
		case "wireguard":
			return "", "", nil, fmt.Errorf("WireGuard peers are configured in the Xray profile, not TapX user credentials")
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

func buildURLXrayShareLink(payload SharePayload) (string, []string, error) {
	if payload.Listener == nil {
		return "", nil, fmt.Errorf("xray share requires a listener")
	}
	protocol := strings.ToLower(payload.XrayProfile.InboundProtocol)
	credential := clientCredential(payload.Client, protocol)
	if credential == "" {
		return "", nil, fmt.Errorf("%s share requires a client credential", protocol)
	}
	host, warnings := shareHost(payload.Listener)
	values := xrayLinkValues(*payload.XrayProfile)
	if flow := inboundClientField(payload.XrayProfile, credential, "flow"); flow != "" {
		values.Set("flow", flow)
	}
	name := firstNonEmpty(payload.Client.Name, payload.Client.Email, payload.Client.ID)
	u := url.URL{
		Scheme:   protocol,
		User:     url.User(credential),
		Host:     net.JoinHostPort(host, strconv.Itoa(int(payload.Listener.BindPort))),
		RawQuery: values.Encode(),
		Fragment: name,
	}
	return u.String(), warnings, nil
}

func buildRawShareLink(payload SharePayload) (string, []string, error) {
	if payload.Listener == nil {
		return "", nil, fmt.Errorf("raw share requires a listener")
	}
	host, warnings := shareHost(payload.Listener)
	values := url.Values{}
	network := "udp"
	security := "none"
	if payload.Transport == model.TransportTCP {
		network = "tcp"
		if payload.RawTCP != nil && payload.RawTCP.TLS.Enabled {
			security = "tls"
			values.Set("sni", payload.RawTCP.TLS.ServerName)
		}
	} else if payload.RawUDP != nil && payload.RawUDP.DTLS.Enabled {
		security = "dtls"
		values.Set("sni", payload.RawUDP.DTLS.ServerName)
	}
	values.Set("network", network)
	values.Set("security", security)
	vkey := ""
	if payload.VKey != nil {
		vkey = payload.VKey.Value
	}
	if vkey == "" && strings.EqualFold(payload.Client.CredentialType, "vkey") {
		vkey = payload.Client.CredentialValue
	}
	if vkey != "" {
		values.Set("vkey", vkey)
	}
	u := url.URL{Scheme: "raw", Host: net.JoinHostPort(host, strconv.Itoa(int(payload.Listener.BindPort))), RawQuery: values.Encode(), Fragment: shareName(payload.Client)}
	return u.String(), warnings, nil
}

func buildVMessShareLink(payload SharePayload) (string, []string, error) {
	host, warnings := shareHost(payload.Listener)
	credential := clientCredential(payload.Client, "vmess")
	if credential == "" {
		return "", nil, fmt.Errorf("vmess share requires a client UUID")
	}
	values := xrayLinkValues(*payload.XrayProfile)
	item := map[string]string{
		"v": "2", "ps": shareName(payload.Client), "add": host,
		"port": strconv.Itoa(int(payload.Listener.BindPort)), "id": credential,
		"aid": "0", "scy": firstNonEmpty(inboundClientField(payload.XrayProfile, credential, "security"), "auto"),
		"net": values.Get("type"), "tls": values.Get("security"),
		"sni": values.Get("sni"), "host": values.Get("host"), "path": values.Get("path"),
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return "", nil, err
	}
	return "vmess://" + base64.RawStdEncoding.EncodeToString(raw), warnings, nil
}

func buildShadowsocksShareLink(payload SharePayload) (string, []string, error) {
	host, warnings := shareHost(payload.Listener)
	password := clientCredential(payload.Client, "shadowsocks")
	if password == "" {
		return "", nil, fmt.Errorf("shadowsocks share requires a password")
	}
	settings := decodeJSONObject(payload.XrayProfile.InboundSettingsJSON)
	method := firstNonEmpty(stringMapValue(settings, "method"), "aes-256-gcm")
	credentials := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
	return fmt.Sprintf("ss://%s@%s#%s", credentials, net.JoinHostPort(host, strconv.Itoa(int(payload.Listener.BindPort))), url.PathEscape(shareName(payload.Client))), warnings, nil
}

func buildHysteriaShareLink(payload SharePayload) (string, []string, error) {
	host, warnings := shareHost(payload.Listener)
	auth := clientCredential(payload.Client, "hysteria")
	if auth == "" {
		return "", nil, fmt.Errorf("hysteria share requires auth")
	}
	values := xrayLinkValues(*payload.XrayProfile)
	u := url.URL{Scheme: "hysteria2", User: url.User(auth), Host: net.JoinHostPort(host, strconv.Itoa(int(payload.Listener.BindPort))), Fragment: shareName(payload.Client)}
	if values.Get("sni") != "" {
		u.RawQuery = url.Values{"sni": []string{values.Get("sni")}}.Encode()
	}
	return u.String(), warnings, nil
}

func inboundClientField(profile *model.XrayProfile, credential, field string) string {
	if profile == nil || credential == "" {
		return ""
	}
	settings := decodeJSONObject(profile.InboundSettingsJSON)
	clients, _ := settings["clients"].([]any)
	for _, value := range clients {
		client, _ := value.(map[string]any)
		if stringMapValue(client, "id") == credential {
			return stringMapValue(client, field)
		}
	}
	return ""
}

func xrayLinkValues(profile model.XrayProfile) url.Values {
	stream := decodeJSONObject(profile.StreamSettingsJSON)
	values := url.Values{}
	network := firstNonEmpty(profile.Network, stringMapValue(stream, "network"), "tcp")
	security := firstNonEmpty(profile.Security, stringMapValue(stream, "security"), "none")
	values.Set("type", network)
	values.Set("security", security)
	transport := objectMapValue(stream, network+"Settings")
	values.Set("host", stringMapValue(transport, "host"))
	values.Set("path", stringMapValue(transport, "path"))
	if network == "grpc" {
		values.Set("serviceName", stringMapValue(transport, "serviceName"))
	}
	securitySettings := objectMapValue(stream, security+"Settings")
	values.Set("sni", firstNonEmpty(stringMapValue(securitySettings, "serverName"), stringMapValue(securitySettings, "dest")))
	values.Set("fp", stringMapValue(securitySettings, "fingerprint"))
	values.Set("pbk", stringMapValue(securitySettings, "publicKey"))
	values.Set("sid", stringMapValue(securitySettings, "shortId"))
	for key := range values {
		if values.Get(key) == "" {
			values.Del(key)
		}
	}
	return values
}

func shareHost(listener *ShareListener) (string, []string) {
	if listener == nil {
		return "", nil
	}
	host := normalizeShareHost(listener.BindHost)
	if host != "" {
		return host, nil
	}
	return "server.example", []string{"listener bind host is wildcard; replace server.example in the share link"}
}

func shareName(client ShareClient) string {
	return firstNonEmpty(client.Name, client.Email, client.ID)
}

func clientCredential(client ShareClient, protocol string) string {
	switch protocol {
	case "vless", "vmess":
		return firstNonEmpty(client.UUID, client.CredentialValue)
	case "trojan", "shadowsocks":
		return firstNonEmpty(client.Password, client.CredentialValue)
	case "hysteria", "hysteria2":
		return firstNonEmpty(client.Auth, client.Password, client.CredentialValue)
	default:
		return client.CredentialValue
	}
}

func decodeJSONObject(value string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(value), &out)
	return out
}

func objectMapValue(value map[string]any, key string) map[string]any {
	item, _ := value[key].(map[string]any)
	return item
}

func stringMapValue(value map[string]any, key string) string {
	item, _ := value[key].(string)
	return item
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeShareHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return ""
	}
	return strings.Trim(host, "[]")
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
