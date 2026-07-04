package model

type DeviceType string

const (
	DeviceTAP DeviceType = "tap"
	DeviceTUN DeviceType = "tun"
)

type Transport string

const (
	TransportXray Transport = "xray"
	TransportTCP  Transport = "tcp"
	TransportUDP  Transport = "udp"
)

type XrayRuntime string

const (
	XrayEmbedded XrayRuntime = "embedded"
	XrayExternal XrayRuntime = "external"
)

type Device struct {
	ID       string
	Enabled  bool
	Name     string
	Type     DeviceType
	IfName   string
	MTU      int
	MSSClamp int
	IPv4CIDR string
	IPv6CIDR string
	Bridge   *BridgeConfig
	Routes   []DeviceRoute
	DNS      *DNSConfig
	Remark   string
}

type BridgeConfig struct {
	Enabled bool
	Name    string
	IfName  string
	MTU     int
}

type DeviceRoute struct {
	Enabled     bool
	Destination string
	Gateway     string
	Source      string
	IfName      string
	Metric      int
	Table       string
}

type DNSConfig struct {
	Enabled       bool
	Nameservers   []string
	SearchDomains []string
	Options       []string
	OutputPath    string
}

type Listener struct {
	ID            string
	Enabled       bool
	Name          string
	BindHost      string
	BindPort      uint16
	Transport     Transport
	XrayProfileID string
	RawUDP        RawUDPSettings
	RawTCP        RawTCPSettings
	Binding       Binding
	Remark        string
}

type Connector struct {
	ID            string
	Enabled       bool
	Name          string
	Remote        string
	Port          uint16
	Transport     Transport
	XrayProfileID string
	RawUDP        RawUDPSettings
	RawTCP        RawTCPSettings
	Binding       Binding
	Remark        string
}

type Client struct {
	ID              string
	Enabled         bool
	Name            string
	Email           string
	ListenerID      string
	CredentialType  string
	CredentialValue string
	Binding         Binding
	AddressID       string
	ExpiresAt       int64
	TrafficCap      uint64
	TrafficResetAt  int64
	TrafficRXOffset uint64
	TrafficTXOffset uint64
	Remark          string
}

type Route struct {
	ID          string
	Enabled     bool
	VKeyID      string
	ListenerID  string
	DeviceID    string
	ConnectorID string
	ClientID    string
	AddressID   string
}

// Binding captures optional advanced-panel knobs. Empty fields mean the feature
// is not enabled and must not add packet-time work after runtime config generation.
type Binding struct {
	VKeyID      string
	ClientID    string
	RouteID     string
	DeviceID    string
	ConnectorID string
	AddressID   string
}

type UDPPeerMode string

const (
	UDPPeerAny   UDPPeerMode = "any"
	UDPPeerFixed UDPPeerMode = "fixed"
	UDPPeerLearn UDPPeerMode = "learn"
)

type RawUDPSettings struct {
	PeerMode        UDPPeerMode
	FixedPeer       string
	BindInterface   string
	BindAddress     string
	ReceiveBuffer   int
	SendBuffer      int
	ReuseAddr       bool
	ReusePort       bool
	KeepAliveSecond int
	Workers         int
	QueueSize       int
	DTLS            RawDTLSSettings
}

type TCPLengthMode string

const (
	TCPLength16 TCPLengthMode = "uint16"
	TCPLength32 TCPLengthMode = "uint32"
)

type RawTCPSettings struct {
	LengthMode      TCPLengthMode
	BindInterface   string
	BindAddress     string
	ReceiveBuffer   int
	SendBuffer      int
	NoDelay         bool
	KeepAliveSecond int
	FastOpen        bool
	ConnectTimeout  int
	ReconnectSecond int
	Workers         int
	ReadBuffer      int
	WriteBuffer     int
	TLS             RawTLSSettings
}

type RawTLSSettings struct {
	Enabled       bool
	CertFile      string
	KeyFile       string
	CAFile        string
	ServerName    string
	ALPN          []string
	MinVersion    string
	MaxVersion    string
	AllowInsecure bool
}

type RawDTLSSettings struct {
	Enabled       bool
	CertFile      string
	KeyFile       string
	CAFile        string
	ServerName    string
	ALPN          []string
	MinVersion    string
	MaxVersion    string
	AllowInsecure bool
	MTU           int
	ReplayWindow  int
}

type VKey struct {
	ID      string
	Enabled bool
	Name    string
	Value   string
	Remark  string
}

type AddressLimit struct {
	ID                string
	Enabled           bool
	Name              string
	DeviceID          string
	ClientID          string
	MACs              []string
	IPv4CIDRs         []string
	IPv6CIDRs         []string
	IPv4Gateway       string
	IPv6Gateway       string
	DNS               []string
	Routes            []string
	AllowDefaultRoute bool
	Remark            string
}

type XrayProfile struct {
	ID                   string
	Enabled              bool
	Name                 string
	Runtime              XrayRuntime
	InboundProtocol      string
	InboundSettingsJSON  string
	OutboundProtocol     string
	OutboundSettingsJSON string
	Network              string
	Security             string
	StreamSettingsJSON   string
	SniffingJSON         string
	MuxJSON              string
	SockoptJSON          string
	FallbacksJSON        string
	RoutingJSON          string
	DNSJSON              string
	PolicyJSON           string
	AdvancedJSON         string
	Remark               string
}

type Settings struct {
	ID                  string
	Enabled             bool
	Name                string
	PanelListen         string
	PanelHTTPS          bool
	PanelCertFile       string
	PanelKeyFile        string
	PanelAuthEnabled    bool
	AdminUsername       string
	AdminPasswordHash   string
	SessionTTLSecond    int
	ExternalXrayPath    string
	LogLevel            string
	StatsIntervalSecond int
	BackupDir           string
	DataDir             string
	OpenWrtBuildTarget  string
	AdvancedJSON        string
	Remark              string
}
