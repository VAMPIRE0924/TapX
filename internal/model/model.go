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
	ID                    string
	Enabled               bool
	Name                  string
	Type                  DeviceType
	IfName                string
	MTU                   int
	MSSClamp              int
	LinkAutoOptimize      bool
	Bridge                *BridgeConfig
	Routes                []DeviceRoute
	TapMode               TapMode
	AccessRole            AccessRole
	DHCP                  *DHCPConfig
	SharedIP              *SharedIPConfig
	TUNDHCP               *TUNDHCPConfig
	OneArmRollbackSeconds int
	Source                string
	Remark                string
}

type TapMode string

const (
	TapModeStandalone  TapMode = "standalone"
	TapModeTransparent TapMode = "transparent"
	TapModeOneArm      TapMode = "one-arm"
	TapModeSharedIP    TapMode = "shared-ip"
)

type AccessRole string

const (
	AccessRoleClient AccessRole = "client"
	AccessRoleServer AccessRole = "server"
)

type DHCPMode string

const (
	DHCPModeOff         DHCPMode = "off"
	DHCPModePassthrough DHCPMode = "passthrough"
	DHCPModeServer      DHCPMode = "server"
	DHCPModeMirror      DHCPMode = "mirror"
)

type DHCPStaticLease struct {
	Name    string
	MAC     string
	Address string
}

type DHCPConfig struct {
	Mode              DHCPMode
	IPv4CIDR          string
	PoolStart         string
	PoolEnd           string
	PrefixLength      int
	Gateway           string
	DNS               []string
	LeaseSeconds      int
	Authoritative     bool
	ConflictDetection bool
	StaticLeases      []DHCPStaticLease
}

type SharedIPRole string

const (
	SharedIPRoleService SharedIPRole = "service"
	SharedIPRoleAccess  SharedIPRole = "access"
)

type FirewallBackend string

const (
	FirewallAuto     FirewallBackend = "auto"
	FirewallNFTables FirewallBackend = "nftables"
	FirewallIPTables FirewallBackend = "iptables"
)

type SharedIPConfig struct {
	Role                SharedIPRole
	UplinkInterface     string
	AddressSource       string
	IPv4CIDR            string
	Gateway             string
	DNS                 []string
	FirewallBackend     FirewallBackend
	HostPortPriority    bool
	TrackAddressChanges bool
	ReservedTCPPorts    []string
	ReservedUDPPorts    []string
	ClientMAC           string
}

type TUNDHCPMode string

const (
	TUNDHCPModeOff    TUNDHCPMode = "off"
	TUNDHCPModeClient TUNDHCPMode = "client"
	TUNDHCPModeServer TUNDHCPMode = "server"
	TUNDHCPModeManual TUNDHCPMode = "manual"
)

type TUNDHCPConfig struct {
	Mode                      TUNDHCPMode
	Protocol                  string
	RelayEnabled              bool
	RelayProtocol             string
	IPv4CIDR                  string
	IPv6CIDR                  string
	PoolStart                 string
	PoolEnd                   string
	IPv6PoolStart             string
	IPv6PoolEnd               string
	Gateway                   string
	DNS                       []string
	OfferedGateway            string
	OfferedDNS                []string
	LeaseSeconds              int
	Authoritative             bool
	ConflictDetection         bool
	RelayDownstreamInterfaces []string
	RelayServers              []string
	MaxHops                   int
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

type Listener struct {
	ID                     string
	Enabled                bool
	Name                   string
	BindHost               string
	BindPort               uint16
	Transport              Transport
	XrayProfileID          string
	RawUDP                 RawUDPSettings
	RawTCP                 RawTCPSettings
	Binding                Binding
	ShareAddressStrategy   string
	ShareAddress           string
	ExpiresAt              int64
	TrafficCap             uint64
	TrafficReset           string
	TrafficResetAt         int64
	TrafficResetGeneration uint64
	TrafficRXOffset        uint64
	TrafficTXOffset        uint64
	Remark                 string
}

type Connector struct {
	ID                     string
	Enabled                bool
	Name                   string
	Remote                 string
	Port                   uint16
	Transport              Transport
	XrayProfileID          string
	RawUDP                 RawUDPSettings
	RawTCP                 RawTCPSettings
	Binding                Binding
	TrafficResetAt         int64
	TrafficResetGeneration uint64
	TrafficRXOffset        uint64
	TrafficTXOffset        uint64
	Remark                 string
	CreatedAt              int64
	UpdatedAt              int64
}

type Client struct {
	ID                     string
	Enabled                bool
	Name                   string
	Email                  string
	ListenerID             string
	ListenerIDs            []string
	UUID                   string
	Password               string
	Auth                   string
	AllowedDeviceIDs       []string
	Binding                Binding
	AddressID              string
	ExpiresAt              int64
	TrafficCap             uint64
	UploadRateLimit        uint64
	DownloadRateLimit      uint64
	TrafficReset           string
	TrafficResetAt         int64
	TrafficResetGeneration uint64
	TrafficRXOffset        uint64
	TrafficTXOffset        uint64
	Remark                 string
	CreatedAt              int64
	UpdatedAt              int64
}

type Route struct {
	ID          string
	Enabled     bool
	Priority    int
	Action      RouteAction
	VKeyID      string
	ListenerID  string
	DeviceID    string
	ConnectorID string
	ClientID    string
	AddressID   string
}

type RouteAction string

const (
	RouteActionBindDevice RouteAction = "bind-device"
	RouteActionAllow      RouteAction = "allow"
	RouteActionDrop       RouteAction = "drop"
)

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

type RawUDPSettings struct {
	KeepAliveSecond int
	Workers         int
	QueueSize       int
	ZeroCopy        bool
	ConnectTimeout  int
	IdleTimeout     int
	DTLS            RawDTLSSettings
}

type TCPLengthMode string

const (
	TCPLength16 TCPLengthMode = "uint16"
	TCPLength32 TCPLengthMode = "uint32"
)

type RawTCPSettings struct {
	LengthMode      TCPLengthMode
	NoDelay         bool
	KeepAliveSecond int
	FastOpen        bool
	ConnectTimeout  int
	ReconnectSecond int
	Workers         int
	QueueSize       int
	ZeroCopy        bool
	IdleTimeout     int
	TLS             RawTLSSettings
}

type RawTLSSettings struct {
	Enabled       bool
	CertFile      string
	KeyFile       string
	ServerName    string
	MinVersion    string
	MaxVersion    string
	AllowInsecure bool
}

type RawDTLSSettings struct {
	Enabled       bool
	CertFile      string
	KeyFile       string
	ServerName    string
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
	SendThrough          string
	TargetStrategy       string
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
	ID                     string
	Enabled                bool
	Name                   string
	PanelName              string
	PanelListen            string
	PanelDomain            string
	PanelBasePath          string
	PanelHTTPS             bool
	PanelCertFile          string
	PanelKeyFile           string
	PanelAuthEnabled       bool
	AdminUsername          string
	AdminPasswordHash      string
	SessionTTLSecond       int
	Timezone               string
	PanelOutbound          string
	ExternalXrayPath       string
	ExternalXrayConfigFile string
	ExternalXrayWorkDir    string
	ExternalXrayArgs       string
	LogLevel               string
	StatsIntervalSecond    int
	BackupDir              string
	DataDir                string
	OpenWrtBuildTarget     string
	AdvancedJSON           string
	Remark                 string
}
