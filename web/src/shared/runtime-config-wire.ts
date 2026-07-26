import type {
  RuntimeConfig,
  TapxAddressLimit,
  TapxBinding,
  TapxClient,
  TapxDevice,
  TapxEndpoint,
  TapxRoute,
  TapxVKey,
  TapxXrayProfile,
} from './api';

export function runtimeConfigForAPI(config: RuntimeConfig): RuntimeConfig {
  return {
    Devices: (config.Devices || []).map(deviceForAPI),
    Listeners: (config.Listeners || []).map(listenerForAPI),
    Connectors: (config.Connectors || []).map(connectorForAPI),
    Clients: (config.Clients || []).map(clientForAPI),
    Routes: (config.Routes || []).map(routeForAPI),
    VKeys: (config.VKeys || []).map(vkeyForAPI),
    Addresses: (config.Addresses || []).map(addressForAPI),
    XrayProfiles: (config.XrayProfiles || []).map(xrayProfileForAPI),
    Settings: config.Settings || [],
  };
}

function bindingForAPI(binding: TapxBinding | undefined): TapxBinding | undefined {
  if (!binding) return undefined;
  return compact({
    VKeyID: binding.VKeyID,
    ClientID: binding.ClientID,
    RouteID: binding.RouteID,
    DeviceID: binding.DeviceID,
    ConnectorID: binding.ConnectorID,
    AddressID: binding.AddressID,
  });
}

function deviceForAPI(device: TapxDevice): TapxDevice {
  return compact({
    ID: device.ID, Enabled: device.Enabled, Name: device.Name, Type: device.Type, IfName: device.IfName,
    MTU: device.MTU, MSSClamp: device.MSSClamp, LinkAutoOptimize: device.LinkAutoOptimize,
    Bridge: device.Bridge && compact({ Enabled: device.Bridge.Enabled, Name: device.Bridge.Name, IfName: device.Bridge.IfName, MTU: device.Bridge.MTU }),
    Routes: device.Routes?.map((route) => compact({
      Enabled: route.Enabled, Destination: route.Destination, Gateway: route.Gateway, Source: route.Source,
      IfName: route.IfName, Metric: route.Metric, Table: route.Table,
    })),
    TapMode: device.TapMode, AccessRole: device.AccessRole,
    DHCP: device.DHCP && compact({
      Mode: device.DHCP.Mode, IPv4CIDR: device.DHCP.IPv4CIDR, PoolStart: device.DHCP.PoolStart,
      PoolEnd: device.DHCP.PoolEnd, PrefixLength: device.DHCP.PrefixLength, Gateway: device.DHCP.Gateway,
      DNS: device.DHCP.DNS, LeaseSeconds: device.DHCP.LeaseSeconds, Authoritative: device.DHCP.Authoritative,
      ConflictDetection: device.DHCP.ConflictDetection,
      StaticLeases: device.DHCP.StaticLeases?.map((lease) => compact({ Name: lease.Name, MAC: lease.MAC, Address: lease.Address })),
    }),
    SharedIP: device.SharedIP && compact({
      Role: device.SharedIP.Role, UplinkInterface: device.SharedIP.UplinkInterface,
      AddressSource: device.SharedIP.AddressSource, IPv4CIDR: device.SharedIP.IPv4CIDR,
      Gateway: device.SharedIP.Gateway, DNS: device.SharedIP.DNS, FirewallBackend: device.SharedIP.FirewallBackend,
      HostPortPriority: device.SharedIP.HostPortPriority, TrackAddressChanges: device.SharedIP.TrackAddressChanges,
      ReservedTCPPorts: device.SharedIP.ReservedTCPPorts, ReservedUDPPorts: device.SharedIP.ReservedUDPPorts,
      ClientMAC: device.SharedIP.ClientMAC,
    }),
    TUNDHCP: device.TUNDHCP && compact({
      Mode: device.TUNDHCP.Mode, Protocol: device.TUNDHCP.Protocol, RelayEnabled: device.TUNDHCP.RelayEnabled,
      RelayProtocol: device.TUNDHCP.RelayProtocol, IPv4CIDR: device.TUNDHCP.IPv4CIDR,
      IPv6CIDR: device.TUNDHCP.IPv6CIDR, PoolStart: device.TUNDHCP.PoolStart, PoolEnd: device.TUNDHCP.PoolEnd,
      IPv6PoolStart: device.TUNDHCP.IPv6PoolStart, IPv6PoolEnd: device.TUNDHCP.IPv6PoolEnd,
      Gateway: device.TUNDHCP.Gateway, DNS: device.TUNDHCP.DNS, OfferedGateway: device.TUNDHCP.OfferedGateway,
      OfferedDNS: device.TUNDHCP.OfferedDNS, LeaseSeconds: device.TUNDHCP.LeaseSeconds,
      Authoritative: device.TUNDHCP.Authoritative, ConflictDetection: device.TUNDHCP.ConflictDetection,
      RelayDownstreamInterfaces: device.TUNDHCP.RelayDownstreamInterfaces,
      RelayServers: device.TUNDHCP.RelayServers, MaxHops: device.TUNDHCP.MaxHops,
    }),
    OneArmRollbackSeconds: device.OneArmRollbackSeconds, Source: device.Source, Remark: device.Remark,
  }) as TapxDevice;
}

function rawUDPForAPI(raw: TapxEndpoint['RawUDP']): TapxEndpoint['RawUDP'] {
  if (!raw) return undefined;
  return compact({
    KeepAliveSecond: raw.KeepAliveSecond, Workers: raw.Workers, QueueSize: raw.QueueSize,
    ZeroCopy: raw.ZeroCopy, ConnectTimeout: raw.ConnectTimeout, IdleTimeout: raw.IdleTimeout,
    DTLS: raw.DTLS && compact({
      Enabled: raw.DTLS.Enabled, CertFile: raw.DTLS.CertFile, KeyFile: raw.DTLS.KeyFile,
      ServerName: raw.DTLS.ServerName, MinVersion: raw.DTLS.MinVersion, MaxVersion: raw.DTLS.MaxVersion,
      AllowInsecure: raw.DTLS.AllowInsecure, MTU: raw.DTLS.MTU, ReplayWindow: raw.DTLS.ReplayWindow,
    }),
  });
}

function rawTCPForAPI(raw: TapxEndpoint['RawTCP']): TapxEndpoint['RawTCP'] {
  if (!raw) return undefined;
  return compact({
    LengthMode: raw.LengthMode, NoDelay: raw.NoDelay, KeepAliveSecond: raw.KeepAliveSecond,
    FastOpen: raw.FastOpen, Workers: raw.Workers, ConnectTimeout: raw.ConnectTimeout,
    ReconnectSecond: raw.ReconnectSecond, QueueSize: raw.QueueSize, ZeroCopy: raw.ZeroCopy,
    IdleTimeout: raw.IdleTimeout,
    TLS: raw.TLS && compact({
      Enabled: raw.TLS.Enabled, CertFile: raw.TLS.CertFile, KeyFile: raw.TLS.KeyFile,
      ServerName: raw.TLS.ServerName, MinVersion: raw.TLS.MinVersion, MaxVersion: raw.TLS.MaxVersion,
      AllowInsecure: raw.TLS.AllowInsecure,
    }),
  });
}

function listenerForAPI(endpoint: TapxEndpoint): TapxEndpoint {
  return compact({
    ID: endpoint.ID, Enabled: endpoint.Enabled, Name: endpoint.Name, BindHost: endpoint.BindHost,
    BindPort: endpoint.BindPort, Transport: endpoint.Transport, XrayProfileID: endpoint.XrayProfileID,
    RawUDP: rawUDPForAPI(endpoint.RawUDP), RawTCP: rawTCPForAPI(endpoint.RawTCP),
    Binding: bindingForAPI(endpoint.Binding), ExpiresAt: endpoint.ExpiresAt, TrafficCap: endpoint.TrafficCap,
    TrafficReset: endpoint.TrafficReset, TrafficResetAt: endpoint.TrafficResetAt,
    TrafficResetGeneration: endpoint.TrafficResetGeneration, TrafficRXOffset: endpoint.TrafficRXOffset,
    TrafficTXOffset: endpoint.TrafficTXOffset, ShareAddressStrategy: endpoint.ShareAddressStrategy,
    ShareAddress: endpoint.ShareAddress, Remark: endpoint.Remark,
  }) as TapxEndpoint;
}

function connectorForAPI(endpoint: TapxEndpoint): TapxEndpoint {
  const record = endpoint as TapxEndpoint & { CreatedAt?: number; UpdatedAt?: number };
  return compact({
    ID: endpoint.ID, Enabled: endpoint.Enabled, Name: endpoint.Name, Remote: endpoint.Remote, Port: endpoint.Port,
    Transport: endpoint.Transport, XrayProfileID: endpoint.XrayProfileID, RawUDP: rawUDPForAPI(endpoint.RawUDP),
    RawTCP: rawTCPForAPI(endpoint.RawTCP), Binding: bindingForAPI(endpoint.Binding),
    TrafficResetAt: endpoint.TrafficResetAt, TrafficResetGeneration: endpoint.TrafficResetGeneration,
    TrafficRXOffset: endpoint.TrafficRXOffset, TrafficTXOffset: endpoint.TrafficTXOffset,
    Remark: endpoint.Remark, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
  }) as TapxEndpoint;
}

function clientForAPI(client: TapxClient): TapxClient {
  const record = client as TapxClient & { CreatedAt?: number; UpdatedAt?: number };
  return compact({
    ID: client.ID, Enabled: client.Enabled, Name: client.Name, Email: client.Email,
    ListenerID: client.ListenerID, ListenerIDs: client.ListenerIDs, UUID: client.UUID,
    Password: client.Password, Auth: client.Auth, AllowedDeviceIDs: client.AllowedDeviceIDs,
    Binding: bindingForAPI(client.Binding), AddressID: client.AddressID, ExpiresAt: client.ExpiresAt,
    TrafficCap: client.TrafficCap, UploadRateLimit: client.UploadRateLimit,
    DownloadRateLimit: client.DownloadRateLimit, TrafficReset: client.TrafficReset,
    TrafficResetAt: client.TrafficResetAt, TrafficResetGeneration: client.TrafficResetGeneration,
    TrafficRXOffset: client.TrafficRXOffset, TrafficTXOffset: client.TrafficTXOffset,
    Remark: client.Remark, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
  }) as TapxClient;
}

function routeForAPI(route: TapxRoute): TapxRoute {
  return compact({
    ID: route.ID, Enabled: route.Enabled, Priority: route.Priority, Action: route.Action,
    VKeyID: route.VKeyID, ListenerID: route.ListenerID, DeviceID: route.DeviceID,
    ConnectorID: route.ConnectorID, ClientID: route.ClientID, AddressID: route.AddressID,
  }) as TapxRoute;
}

function vkeyForAPI(vkey: TapxVKey): TapxVKey {
  return compact({ ID: vkey.ID, Enabled: vkey.Enabled, Name: vkey.Name, Value: vkey.Value, Remark: vkey.Remark }) as TapxVKey;
}

function addressForAPI(address: TapxAddressLimit): TapxAddressLimit {
  return compact({
    ID: address.ID, Enabled: address.Enabled, Name: address.Name, DeviceID: address.DeviceID,
    ClientID: address.ClientID, MACs: address.MACs, IPv4CIDRs: address.IPv4CIDRs,
    IPv6CIDRs: address.IPv6CIDRs, IPv4Gateway: address.IPv4Gateway, IPv6Gateway: address.IPv6Gateway,
    DNS: address.DNS, Routes: address.Routes, AllowDefaultRoute: address.AllowDefaultRoute, Remark: address.Remark,
  }) as TapxAddressLimit;
}

function xrayProfileForAPI(profile: TapxXrayProfile): TapxXrayProfile {
  return compact({
    ID: profile.ID, Enabled: profile.Enabled, Name: profile.Name, Runtime: profile.Runtime,
    InboundProtocol: profile.InboundProtocol, InboundSettingsJSON: profile.InboundSettingsJSON,
    OutboundProtocol: profile.OutboundProtocol, OutboundSettingsJSON: profile.OutboundSettingsJSON,
    SendThrough: profile.SendThrough, TargetStrategy: profile.TargetStrategy, Network: profile.Network,
    Security: profile.Security, StreamSettingsJSON: profile.StreamSettingsJSON, SniffingJSON: profile.SniffingJSON,
    MuxJSON: profile.MuxJSON, SockoptJSON: profile.SockoptJSON, FallbacksJSON: profile.FallbacksJSON,
    RoutingJSON: profile.RoutingJSON, DNSJSON: profile.DNSJSON, PolicyJSON: profile.PolicyJSON,
    AdvancedJSON: profile.AdvancedJSON, Remark: profile.Remark,
  }) as TapxXrayProfile;
}

function compact<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as T;
}
