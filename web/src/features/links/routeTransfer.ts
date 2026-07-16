import type { RuntimeConfig, TapxAddressLimit, TapxRoute } from '../../shared/api';
import type { TranslationKey, TranslationValues } from '../../i18n/dictionaries';
import { nodeIDOf, type NodeOwned } from '../nodes/managedConfig';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export interface RouteTransferBundle {
  Version: 1;
  Routes: TapxRoute[];
  Addresses: TapxAddressLimit[];
}

export function buildRouteTransferBundle(config: RuntimeConfig): RouteTransferBundle {
  const routes = (config.Routes || []).map((route) => ({ ...route }));
  const referencedAddressIDs = new Set(routes.flatMap((route) => (
    route.AddressID ? [`${nodeIDOf(route)}:${route.AddressID}`] : []
  )));
  const addresses = (config.Addresses || [])
    .filter((address) => referencedAddressIDs.has(`${nodeIDOf(address)}:${address.ID}`))
    .map((address) => cloneAddress(address, address.ID, (address as NodeOwned).ManagedNodeID));
  return { Version: 1, Routes: routes, Addresses: addresses };
}

export function importRouteTransferBundle(value: unknown, current: RuntimeConfig, targetNodeID = 'local', t?: Translate): {
  routes: TapxRoute[];
  addresses: TapxAddressLimit[];
} {
  const source = parseSource(value, t);
  const routeIDs = new Set((current.Routes || []).filter((route) => nodeIDOf(route) === targetNodeID).map((route) => route.ID));
  const addressIDs = new Set((current.Addresses || []).filter((address) => nodeIDOf(address) === targetNodeID).map((address) => address.ID));
  const sourceAddresses = new Map(source.addresses.map((address) => [`${nodeIDOf(address)}:${address.ID}`, address]));
  const importedAddresses: TapxAddressLimit[] = [];
  const addressRemap = new Map<string, string>();

  const routes = source.routes.map((route, index) => {
    const sourceNodeID = nodeIDOf(route);
    const nextRouteID = uniqueID(route.ID, routeIDs, 'route-import-' + (index + 1));
    routeIDs.add(nextRouteID);

    let nextAddressID = '';
    if (route.AddressID) {
      const sourceAddress = sourceAddresses.get(`${sourceNodeID}:${route.AddressID}`);
      if (sourceAddress) {
        nextAddressID = addressRemap.get(route.AddressID) || uniqueID(route.AddressID, addressIDs, 'addr-' + nextRouteID);
        if (!addressRemap.has(route.AddressID)) {
          addressRemap.set(route.AddressID, nextAddressID);
          addressIDs.add(nextAddressID);
          importedAddresses.push(cloneAddress(sourceAddress, nextAddressID, targetNodeID));
        }
      } else if ((current.Addresses || []).some((address) => address.ID === route.AddressID && nodeIDOf(address) === targetNodeID)) {
        nextAddressID = route.AddressID;
      } else {
        throw new Error(t ? t('link.importMissingAddress', { route: route.ID, address: route.AddressID }) : `Link binding ${route.ID} references missing address limit ${route.AddressID}`);
      }
    }

    return { ...route, ID: nextRouteID, AddressID: nextAddressID, ...managedNodeField(targetNodeID) } as TapxRoute & NodeOwned;
  });

  return {
    routes,
    addresses: [...(current.Addresses || []), ...importedAddresses],
  };
}

function parseSource(value: unknown, t?: Translate): { routes: TapxRoute[]; addresses: TapxAddressLimit[] } {
  if (Array.isArray(value)) return { routes: value.filter(isRouteRecord), addresses: [] };
  if (!value || typeof value !== 'object') throw new Error(t ? t('link.rulesNotFound') : 'No link-binding rule array was found');
  const object = value as {
    Routes?: unknown;
    routes?: unknown;
    Addresses?: unknown;
    addresses?: unknown;
    config?: { Routes?: unknown; Addresses?: unknown };
  };
  const routeValue = object.Routes ?? object.routes ?? object.config?.Routes;
  if (!Array.isArray(routeValue)) throw new Error(t ? t('link.rulesNotFound') : 'No link-binding rule array was found');
  const addressValue = object.Addresses ?? object.addresses ?? object.config?.Addresses;
  return {
    routes: routeValue.filter(isRouteRecord),
    addresses: Array.isArray(addressValue) ? addressValue.filter(isAddressRecord) : [],
  };
}

function uniqueID(preferred: string, existing: Set<string>, fallback: string): string {
  const base = preferred.trim() || fallback;
  if (!existing.has(base)) return base;
  let suffix = 2;
  while (existing.has(base + '-' + suffix)) suffix += 1;
  return base + '-' + suffix;
}

function cloneAddress(address: TapxAddressLimit, id: string, managedNodeID?: string): TapxAddressLimit & NodeOwned {
  return {
    ...address,
    ID: id,
    ...managedNodeField(managedNodeID || ''),
    IPv4CIDRs: [...(address.IPv4CIDRs || [])],
    IPv6CIDRs: [...(address.IPv6CIDRs || [])],
    MACs: [...(address.MACs || [])],
  };
}

function managedNodeField(managedNodeID: string): NodeOwned {
  return managedNodeID && managedNodeID !== 'local' ? { ManagedNodeID: managedNodeID } : {};
}

function isRouteRecord(value: unknown): value is TapxRoute {
  return !!value && typeof value === 'object' && typeof (value as { ID?: unknown }).ID === 'string';
}

function isAddressRecord(value: unknown): value is TapxAddressLimit {
  return !!value && typeof value === 'object' && typeof (value as { ID?: unknown }).ID === 'string';
}
