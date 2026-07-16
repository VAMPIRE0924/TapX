import type { RuntimeConfig, TapxAddressLimit, TapxClient, TapxVKey } from '../../shared/api';
import type { TranslationKey, TranslationValues } from '../../i18n/dictionaries';
import { nodeIDOf, type NodeOwned } from '../nodes/managedConfig';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export type UserTransferBundle = {
  Version: 1;
  Clients: TapxClient[];
  Addresses: TapxAddressLimit[];
  VKeys: TapxVKey[];
};

export function exportUserBundle(clients: TapxClient[], config: RuntimeConfig): UserTransferBundle {
  const addressIDs = new Set(clients.flatMap((item) => [item.AddressID, item.Binding?.AddressID]
    .filter(Boolean)
    .map((id) => `${nodeIDOf(item)}:${id}`)));
  const vkeyIDs = new Set(clients.flatMap((item) => (
    item.Binding?.VKeyID ? [`${nodeIDOf(item)}:${item.Binding.VKeyID}`] : []
  )));
  return {
    Version: 1,
    Clients: clients.map(sanitizeUserCredentials),
    Addresses: (config.Addresses || []).filter((item) => addressIDs.has(`${nodeIDOf(item)}:${item.ID}`)),
    VKeys: (config.VKeys || []).filter((item) => vkeyIDs.has(`${nodeIDOf(item)}:${item.ID}`)),
  };
}

export function importUserBundle(value: string, current: RuntimeConfig, targetNodeID = 'local', t?: Translate): { clients: TapxClient[]; addresses: TapxAddressLimit[]; vkeys: TapxVKey[]; skipped: number } {
  const parsed = JSON.parse(value) as unknown;
  const bundle = normalizeBundle(parsed, t);
  const currentClients = [...(current.Clients || [])];
  const currentAddresses = [...(current.Addresses || [])];
  const currentVKeys = [...(current.VKeys || [])];
  const clientIDs = new Set(currentClients.filter((item) => nodeIDOf(item) === targetNodeID).map((item) => item.ID));
  const emails = new Set(currentClients.filter((item) => nodeIDOf(item) === targetNodeID)
    .map((item) => String(item.Email || '').trim().toLowerCase()).filter(Boolean));
  const addressIDs = new Set(currentAddresses.filter((item) => nodeIDOf(item) === targetNodeID).map((item) => item.ID));
  const vkeyIDs = new Set(currentVKeys.filter((item) => nodeIDOf(item) === targetNodeID).map((item) => item.ID));
  const incomingAddresses = new Map(bundle.Addresses.map((item) => [`${nodeIDOf(item)}:${item.ID}`, item]));
  const incomingVKeys = new Map(bundle.VKeys.map((item) => [`${nodeIDOf(item)}:${item.ID}`, item]));
  let skipped = 0;

  for (const source of bundle.Clients) {
    const email = String(source.Email || '').trim().toLowerCase();
    if (!source.ID || clientIDs.has(source.ID) || (email && emails.has(email))) {
      skipped += 1;
      continue;
    }
    const sourceNodeID = nodeIDOf(source);
    const client = { ...sanitizeUserCredentials(source), ...managedNodeField(targetNodeID) } as TapxClient & NodeOwned;
    const oldAddressID = client.AddressID || client.Binding?.AddressID || '';
    if (oldAddressID) {
      const importedAddress = incomingAddresses.get(`${sourceNodeID}:${oldAddressID}`);
      if (!importedAddress) throw new Error(t ? t('user.importMissingAddress', { user: client.ID, address: oldAddressID }) : `User ${client.ID} references missing address limit ${oldAddressID}`);
      const nextAddressID = uniqueID(oldAddressID, addressIDs);
      addressIDs.add(nextAddressID);
      currentAddresses.push({ ...importedAddress, ID: nextAddressID, ClientID: client.ID, ...managedNodeField(targetNodeID) } as TapxAddressLimit & NodeOwned);
      client.AddressID = nextAddressID;
      client.Binding = { ...client.Binding, AddressID: nextAddressID };
    }
    const oldVKeyID = client.Binding?.VKeyID || '';
    if (oldVKeyID) {
      const importedVKey = incomingVKeys.get(`${sourceNodeID}:${oldVKeyID}`);
      if (!importedVKey) throw new Error(t ? t('user.importMissingVkey', { user: client.ID, vkey: oldVKeyID }) : `User ${client.ID} references missing vKey ${oldVKeyID}`);
      const nextVKeyID = uniqueID(oldVKeyID, vkeyIDs);
      vkeyIDs.add(nextVKeyID);
      currentVKeys.push({ ...importedVKey, ID: nextVKeyID, ...managedNodeField(targetNodeID) } as TapxVKey & NodeOwned);
      client.Binding = { ...client.Binding, VKeyID: nextVKeyID };
    }
    currentClients.push(client);
    clientIDs.add(client.ID);
    if (email) emails.add(email);
  }
  return { clients: currentClients, addresses: currentAddresses, vkeys: currentVKeys, skipped };
}

export function sanitizeUserCredentials(client: TapxClient): TapxClient {
  const sanitized = structuredClone(client);
  delete sanitized.Security;
  delete sanitized.ReverseTag;
  delete sanitized.Flow;
  delete sanitized.WireguardPrivateKey;
  delete sanitized.WireguardPublicKey;
  delete sanitized.WireguardPreSharedKey;
  delete sanitized.WireguardAllowedIPs;
  return sanitized;
}

function normalizeBundle(value: unknown, t?: Translate): UserTransferBundle {
  if (Array.isArray(value)) return { Version: 1, Clients: value as TapxClient[], Addresses: [], VKeys: [] };
  if (!value || typeof value !== 'object') throw new Error(t ? t('user.importBundleRequired') : 'Imported content must be a user array or user export bundle');
  const object = value as Partial<UserTransferBundle>;
  if (!Array.isArray(object.Clients)) throw new Error(t ? t('user.importClientsRequired') : 'User export bundle is missing a Clients array');
  return {
    Version: 1,
    Clients: object.Clients,
    Addresses: Array.isArray(object.Addresses) ? object.Addresses : [],
    VKeys: Array.isArray(object.VKeys) ? object.VKeys : [],
  };
}

function uniqueID(base: string, used: Set<string>): string {
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

function managedNodeField(managedNodeID: string): NodeOwned {
  return managedNodeID && managedNodeID !== 'local' ? { ManagedNodeID: managedNodeID } : {};
}
