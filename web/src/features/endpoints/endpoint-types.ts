import type { TapxBinding } from '../../shared/api';

export type EndpointRuntimeMode = 'embedded-xray' | 'external-xray' | 'tapx';
export type DeviceBindMode = 'existing' | 'autoCreate';
export type AddressAssignMode = 'auto' | 'manual';
export type InterfaceType = 'tun' | 'tap';

export type EndpointDeviceBinding = TapxBinding & {
  AutoCreateDevice?: boolean;
  DeviceBindMode?: DeviceBindMode;
  InterfaceType?: InterfaceType;
  DeviceName?: string;
  AddressConfigEnabled?: boolean;
  AddressAssignMode?: AddressAssignMode;
  IPv4CIDR?: string;
  IPv6CIDR?: string;
  Gateway?: string;
  MTU?: number;
  MSSClamp?: number;
  LinkAutoOptimize?: boolean;
};

export const tapxProtocolOptions = [
  { value: 'raw-tcp', label: 'Raw TCP' },
  { value: 'raw-udp', label: 'Raw UDP' },
];
