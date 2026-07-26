import type { TapxBinding, TapxDevice, TapxEndpoint } from '../../shared/api';
import { safeID, uniqueID } from '../../shared/ids';
import type { AddressAssignMode, DeviceBindMode, EndpointDeviceBinding } from './endpoint-types';

type NodeOwned = { ManagedNodeID?: string };

function nodeIDOf(value: NodeOwned | undefined): string {
  return value?.ManagedNodeID || 'local';
}

export class DeviceTypeConflictError extends Error {
  constructor(
    readonly interfaceName: string,
    readonly existingType: 'tun' | 'tap',
    readonly requestedType: 'tun' | 'tap',
  ) {
    super(`Device ${interfaceName} already exists as ${existingType.toUpperCase()}`);
    this.name = 'DeviceTypeConflictError';
  }
}

export function deviceTypeConflictValues(error: DeviceTypeConflictError) {
  return {
    interfaceName: error.interfaceName,
    existingType: error.existingType.toUpperCase(),
    requestedType: error.requestedType.toUpperCase(),
  };
}

export function hydrateSavedDeviceBinding(binding: EndpointDeviceBinding | undefined, device?: TapxDevice): EndpointDeviceBinding | undefined {
  if (!binding?.DeviceID || !device) return {
    ...binding,
    DeviceBindingEnabled: hasEndpointDeviceBinding(binding),
  };
  const addressMode = device.TUNDHCP?.Mode;
  const addressConfigEnabled = device.Type === 'tun' && ['client', 'server', 'manual'].includes(addressMode || '');
  return {
    ...binding,
    DeviceBindingEnabled: true,
    DeviceBindMode: 'existing',
    AutoCreateDevice: false,
    InterfaceType: device.Type === 'tap' ? 'tap' : 'tun',
    DeviceName: device.IfName || device.Name || device.ID,
    MTU: device.MTU,
    MSSClamp: device.MSSClamp,
    LinkAutoOptimize: device.LinkAutoOptimize,
    AddressConfigEnabled: addressConfigEnabled,
    AddressAssignMode: addressMode === 'client' ? 'auto' : 'manual',
    IPv4CIDR: device.TUNDHCP?.IPv4CIDR,
    IPv6CIDR: device.TUNDHCP?.IPv6CIDR,
    Gateway: device.TUNDHCP?.Gateway,
  };
}

export function normalizeDeviceBinding(
  binding: EndpointDeviceBinding | undefined,
  defaults: { mode: DeviceBindMode; addressMode: AddressAssignMode },
): EndpointDeviceBinding {
  const enabled = binding?.DeviceBindingEnabled ?? hasEndpointDeviceBinding(binding);
  if (!enabled) return { ...binding, DeviceBindingEnabled: false };
  const mode = binding?.DeviceBindMode || defaults.mode;
  return {
    ...binding,
    DeviceBindingEnabled: true,
    DeviceBindMode: mode,
    AutoCreateDevice: mode === 'autoCreate',
    InterfaceType: binding?.InterfaceType === 'tap' ? 'tap' : 'tun',
    AddressConfigEnabled: mode === 'autoCreate' && binding?.AddressConfigEnabled === true,
    AddressAssignMode: binding?.AddressAssignMode === 'auto' || binding?.AddressAssignMode === 'manual'
      ? binding.AddressAssignMode
      : defaults.addressMode,
    LinkAutoOptimize: mode === 'autoCreate' && binding?.LinkAutoOptimize === true,
    MSSClamp: mode === 'autoCreate' && binding?.LinkAutoOptimize === true ? 0 : binding?.MSSClamp,
  };
}

export function materializeEndpointAutoDevice<T extends TapxEndpoint & { Binding?: EndpointDeviceBinding }>(
  endpoint: T,
  devices: TapxDevice[],
  options: {
    role: 'listener' | 'connector';
    defaultMode: DeviceBindMode;
    defaultAddressMode: AddressAssignMode;
  },
): { endpoint: T; devices: TapxDevice[] } {
  const binding = normalizeDeviceBinding(endpoint.Binding, { mode: options.defaultMode, addressMode: options.defaultAddressMode });
  if (binding.DeviceBindingEnabled === false) {
    return { endpoint: { ...endpoint, Binding: withoutDeviceBinding(binding) }, devices };
  }
  if (binding.DeviceBindMode !== 'autoCreate') {
    return { endpoint: { ...endpoint, Binding: persistedBinding(binding) }, devices };
  }

  const ifName = (binding.DeviceName || '').trim();
  if (!ifName) return { endpoint: { ...endpoint, Binding: persistedBinding(binding) }, devices };

  const endpointOwner = nodeIDOf(endpoint as NodeOwned);
  const explicitOwner = (endpoint as NodeOwned).ManagedNodeID;
  const existing = devices.find((device) => nodeIDOf(device as NodeOwned) === endpointOwner
    && (device.IfName === ifName || device.Name === ifName || device.ID === ifName));
  const requestedType = binding.InterfaceType === 'tap' ? 'tap' : 'tun';
  const existingType = existing?.Type === 'tap' ? 'tap' : 'tun';
  if (existing && existingType !== requestedType) {
    throw new DeviceTypeConflictError(ifName, existingType, requestedType);
  }
  const id = existing?.ID || uniqueID(
    `device-${safeID(ifName)}`,
    new Set(devices.filter((item) => nodeIDOf(item as NodeOwned) === endpointOwner).map((item) => item.ID)),
  );
  const nextDevice: TapxDevice = existing ? {
    ...existing,
  } : {
    ...(explicitOwner ? { ManagedNodeID: explicitOwner } : {}),
    ID: id,
    Enabled: true,
    Name: ifName,
    Type: requestedType,
    IfName: ifName,
    MTU: binding.MTU ?? 1500,
    MSSClamp: binding.LinkAutoOptimize ? 0 : binding.MSSClamp,
    LinkAutoOptimize: binding.LinkAutoOptimize === true,
    AccessRole: options.role === 'listener' ? 'server' : 'client',
    TUNDHCP: requestedType === 'tun' && binding.AddressConfigEnabled ? {
      Mode: options.role === 'connector' && binding.AddressAssignMode === 'auto' ? 'client' : 'manual',
      Protocol: addressProtocol(binding.IPv4CIDR, binding.IPv6CIDR),
      IPv4CIDR: binding.AddressAssignMode !== 'auto' ? binding.IPv4CIDR : undefined,
      IPv6CIDR: binding.AddressAssignMode !== 'auto' ? binding.IPv6CIDR : undefined,
      Gateway: binding.AddressAssignMode !== 'auto' ? binding.Gateway : undefined,
    } : undefined,
    Source: options.role === 'listener' ? 'listener-auto' : 'connector-auto',
    Remark: `tapx:${options.role}-device:${endpoint.Name || endpoint.ID}`,
  };
  const nextDevices = existing
    ? devices.map((item) => (item.ID === id && nodeIDOf(item as NodeOwned) === endpointOwner ? nextDevice : item))
    : [...devices, nextDevice];

  return {
    endpoint: {
      ...endpoint,
      Binding: persistedBinding({ ...binding, DeviceID: id }),
    },
    devices: nextDevices,
  };
}

export function hasEndpointDeviceBinding(binding: EndpointDeviceBinding | undefined): boolean {
  return Boolean(binding?.DeviceID || binding?.DeviceName || binding?.AutoCreateDevice);
}

function withoutDeviceBinding(binding: EndpointDeviceBinding): EndpointDeviceBinding {
  const next = { ...binding };
  delete next.DeviceBindingEnabled;
  delete next.DeviceID;
  delete next.DeviceBindMode;
  delete next.AutoCreateDevice;
  delete next.InterfaceType;
  delete next.DeviceName;
  delete next.AddressConfigEnabled;
  delete next.AddressAssignMode;
  delete next.IPv4CIDR;
  delete next.IPv6CIDR;
  delete next.Gateway;
  delete next.MTU;
  delete next.MSSClamp;
  delete next.LinkAutoOptimize;
  return next;
}

function persistedBinding(binding: EndpointDeviceBinding): TapxBinding {
  return Object.fromEntries(Object.entries({
    VKeyID: binding.VKeyID,
    ClientID: binding.ClientID,
    RouteID: binding.RouteID,
    DeviceID: binding.DeviceID,
    ConnectorID: binding.ConnectorID,
    AddressID: binding.AddressID,
  }).filter(([, value]) => Boolean(value))) as TapxBinding;
}

function addressProtocol(ipv4?: string, ipv6?: string): 'ipv4' | 'ipv6' | 'dual' {
  if (ipv4?.trim() && ipv6?.trim()) return 'dual';
  if (ipv6?.trim()) return 'ipv6';
  return 'ipv4';
}
