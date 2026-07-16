import type { TapxBinding, TapxClient, TapxEndpoint, TapxRoute, TapxVKey } from '../../shared/api';
import { safeID, uniqueID } from '../../shared/ids';
import type { EndpointDeviceBinding } from './endpoint-types';

export type ConnectorVKeyRecord = TapxEndpoint & {
  RuntimeMode?: string;
  VKey?: string;
  Binding?: EndpointDeviceBinding;
};

export type VKeyReferences = {
  listeners: TapxEndpoint[];
  connectors: TapxEndpoint[];
  clients: TapxClient[];
  routes: TapxRoute[];
};

type ExcludedOwner = { connectorID?: string; clientID?: string };
type NodeOwned = { ManagedNodeID?: string };

function nodeIDOf(value: NodeOwned | undefined): string {
  return value?.ManagedNodeID || 'local';
}

export function materializeConnectorVKey<T extends ConnectorVKeyRecord>(
  connector: T,
  vkeys: TapxVKey[],
  references: VKeyReferences,
): { connector: T & ConnectorVKeyRecord; vkeys: TapxVKey[] } {
  const materialized = materializeVKey({
    binding: connector.Binding,
    value: connector.RuntimeMode === 'tapx' ? connector.VKey : '',
    baseID: `vkey-${safeID(connector.Name || connector.ID)}`,
    displayName: connector.Name || connector.ID,
    remarkPrefix: 'connector',
    vkeys,
    references,
    excluded: { connectorID: connector.ID },
    owner: (connector as NodeOwned).ManagedNodeID,
  });
  return {
    connector: { ...connector, VKey: materialized.value, Binding: materialized.binding },
    vkeys: materialized.vkeys,
  };
}

export function materializeClientVKey<T extends TapxClient>(
  client: T,
  vkeys: TapxVKey[],
  references: VKeyReferences,
): { client: T; vkeys: TapxVKey[] } {
  const materialized = materializeVKey({
    binding: client.Binding,
    value: client.VKey,
    baseID: `vkey-${safeID(client.ID)}`,
    displayName: client.Name || client.Email || client.ID,
    remarkPrefix: 'user',
    vkeys,
    references,
    excluded: { clientID: client.ID },
    owner: (client as NodeOwned).ManagedNodeID,
  });
  return {
    client: { ...client, VKey: materialized.value, Binding: materialized.binding },
    vkeys: materialized.vkeys,
  };
}

function materializeVKey({
  binding: sourceBinding,
  value: sourceValue,
  baseID,
  displayName,
  remarkPrefix,
  vkeys,
  references,
  excluded,
  owner,
}: {
  binding?: TapxBinding;
  value?: string;
  baseID: string;
  displayName: string;
  remarkPrefix: 'connector' | 'user';
  vkeys: TapxVKey[];
  references: VKeyReferences;
  excluded: ExcludedOwner;
  owner?: string;
}): { binding: TapxBinding; value: string; vkeys: TapxVKey[] } {
  const ownerID = owner || 'local';
  const binding = { ...sourceBinding };
  const previousID = binding.VKeyID || '';
  const value = (sourceValue || '').trim();
  if (!value) {
    return {
      binding: { ...binding, VKeyID: '' },
      value: '',
      vkeys: removeOrphan(vkeys, previousID, references, excluded, ownerID),
    };
  }

  const sameValue = vkeys.find((item) => nodeIDOf(item as NodeOwned) === ownerID && item.Value === value);
  if (sameValue) {
    return {
      binding: { ...binding, VKeyID: sameValue.ID },
      value,
      vkeys: previousID === sameValue.ID ? vkeys : removeOrphan(vkeys, previousID, references, excluded, ownerID),
    };
  }

  const previous = previousID ? vkeys.find((item) => item.ID === previousID && nodeIDOf(item as NodeOwned) === ownerID) : undefined;
  const canReusePrevious = Boolean(previous && !isVKeyReferenced(previousID, references, excluded, ownerID));
  const id = canReusePrevious ? previousID : uniqueID(
    baseID,
    new Set(vkeys.filter((item) => nodeIDOf(item as NodeOwned) === ownerID).map((item) => item.ID)),
  );
  const nextVKey: TapxVKey = {
    ...(canReusePrevious ? previous : undefined),
    ...(owner ? { ManagedNodeID: owner } : {}),
    ID: id,
    Enabled: true,
    Name: canReusePrevious && previous?.Name ? previous.Name : `${displayName} vKey`,
    Value: value,
    Remark: canReusePrevious && previous?.Remark ? previous.Remark : `tapx:${remarkPrefix}-vkey:${displayName}`,
  };
  return {
    binding: { ...binding, VKeyID: id },
    value,
    vkeys: canReusePrevious
      ? vkeys.map((item) => (item.ID === id && nodeIDOf(item as NodeOwned) === ownerID ? nextVKey : item))
      : [...removeOrphan(vkeys, previousID, references, excluded, ownerID), nextVKey],
  };
}

function removeOrphan(vkeys: TapxVKey[], id: string, references: VKeyReferences, excluded: ExcludedOwner, ownerID: string): TapxVKey[] {
  if (!id || isVKeyReferenced(id, references, excluded, ownerID)) return vkeys;
  return vkeys.filter((item) => item.ID !== id || nodeIDOf(item as NodeOwned) !== ownerID);
}

function isVKeyReferenced(id: string, references: VKeyReferences, excluded: ExcludedOwner, ownerID: string): boolean {
  if (!id) return false;
  if (references.listeners.some((item) => nodeIDOf(item as NodeOwned) === ownerID && item.Binding?.VKeyID === id)) return true;
  if (references.connectors.some((item) => nodeIDOf(item as NodeOwned) === ownerID && item.ID !== excluded.connectorID && item.Binding?.VKeyID === id)) return true;
  if (references.clients.some((item) => nodeIDOf(item as NodeOwned) === ownerID && item.ID !== excluded.clientID && item.Binding?.VKeyID === id)) return true;
  return references.routes.some((item) => nodeIDOf(item as NodeOwned) === ownerID && item.VKeyID === id);
}
