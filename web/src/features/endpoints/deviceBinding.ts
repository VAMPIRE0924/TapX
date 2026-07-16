import type { TapxDevice, TapxEndpoint } from '../../shared/api';
import { safeID, uniqueID } from '../../shared/ids';
import { unixSeconds } from '../../shared/time';
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
  if (!binding?.DeviceID || !device) return binding;
  return {
    ...binding,
    DeviceBindMode: 'existing',
    AutoCreateDevice: false,
    InterfaceType: device.Type === 'tap' ? 'tap' : 'tun',
    DeviceName: device.IfName || device.Name || device.ID,
    MTU: device.MTU,
    MSSClamp: device.MSSClamp,
    LinkAutoOptimize: device.LinkAutoOptimize,
    AddressConfigEnabled: device.AddressConfigEnabled,
    AddressAssignMode: device.AddressAssignMode,
    IPv4CIDR: device.IPv4CIDR,
    IPv6CIDR: device.IPv6CIDR,
    Gateway: device.Gateway,
  };
}

export function normalizeDeviceBinding(
  binding: EndpointDeviceBinding | undefined,
  defaults: { mode: DeviceBindMode; addressMode: AddressAssignMode },
): EndpointDeviceBinding {
  const mode = binding?.DeviceBindMode || defaults.mode;
  return {
    ...binding,
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
    now?: number;
  },
): { endpoint: T; devices: TapxDevice[] } {
  const binding = normalizeDeviceBinding(endpoint.Binding, { mode: options.defaultMode, addressMode: options.defaultAddressMode });
  if (binding.DeviceBindMode !== 'autoCreate') return { endpoint: { ...endpoint, Binding: binding }, devices };

  const ifName = (binding.DeviceName || '').trim();
  if (!ifName) return { endpoint: { ...endpoint, Binding: binding }, devices };

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
  const endpointName = endpoint.Name || endpoint.ID;
  const linkedIDs = new Set(options.role === 'listener' ? existing?.LinkedListenerIDs : existing?.LinkedConnectorIDs);
  const linkedNames = new Set(options.role === 'listener' ? existing?.LinkedListenerNames : existing?.LinkedConnectorNames);
  if (endpoint.ID) linkedIDs.add(endpoint.ID);
  if (endpointName) linkedNames.add(endpointName);

  const linkedFields = options.role === 'listener'
    ? { LinkedListenerIDs: Array.from(linkedIDs), LinkedListenerNames: Array.from(linkedNames) }
    : { LinkedConnectorIDs: Array.from(linkedIDs), LinkedConnectorNames: Array.from(linkedNames) };
  const nextDevice: TapxDevice = existing ? {
    ...existing,
    ...linkedFields,
    UpdatedAt: options.now ?? unixSeconds(),
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
    AddressConfigEnabled: binding.AddressConfigEnabled,
    AddressAssignMode: binding.AddressAssignMode,
    IPv4CIDR: binding.AddressConfigEnabled && binding.AddressAssignMode !== 'auto' ? binding.IPv4CIDR : undefined,
    IPv6CIDR: binding.AddressConfigEnabled && binding.AddressAssignMode !== 'auto' ? binding.IPv6CIDR : undefined,
    Gateway: binding.AddressConfigEnabled && binding.AddressAssignMode !== 'auto' ? binding.Gateway : undefined,
    Source: options.role === 'listener' ? 'listener-auto' : 'connector-auto',
    ...linkedFields,
    UpdatedAt: options.now ?? unixSeconds(),
    Remark: `tapx:${options.role}-device:${endpointName}`,
  };
  const nextDevices = existing
    ? devices.map((item) => (item.ID === id && nodeIDOf(item as NodeOwned) === endpointOwner ? nextDevice : item))
    : [...devices, nextDevice];

  return {
    endpoint: {
      ...endpoint,
      Binding: {
        ...binding,
        DeviceID: id,
        DeviceBindMode: 'existing',
        AutoCreateDevice: false,
      },
    },
    devices: nextDevices,
  };
}
