import {
  applyRuntimeConfig,
  getStats,
  getRuntimeConfig,
  saveRuntimeConfig,
  type ClientQuotaState,
  type RuntimeConfig,
  type StatsBucket,
  type StatsCounters,
  type StatsReport,
} from '../../shared/api';
import {
  applyManagedNodeConfig,
  getManagedNodeConfig,
  loadManagedNodes,
  saveManagedNodeConfig,
} from './nodeRegistry';

export const LOCAL_NODE_ID = 'local';
export const ALL_NODE_ID = 'all';

export interface NodeOwned {
  ManagedNodeID?: string;
}

export interface ManagedConfigObject extends NodeOwned {
  ID: string;
}

export type ManagedStatsBucket = StatsBucket & NodeOwned;
export type ManagedClientQuotaState = ClientQuotaState & NodeOwned;
export type ManagedStatsReport = Omit<StatsReport, 'byEndpoint' | 'byClient' | 'clients'> & {
  byEndpoint?: ManagedStatsBucket[];
  byClient?: ManagedStatsBucket[];
  clients?: ManagedClientQuotaState[];
};

type ConfigListKey = 'Devices' | 'Listeners' | 'Connectors' | 'Clients' | 'Routes' | 'VKeys' | 'Addresses' | 'XrayProfiles';

const configListKeys: ConfigListKey[] = [
  'Devices',
  'Listeners',
  'Connectors',
  'Clients',
  'Routes',
  'VKeys',
  'Addresses',
  'XrayProfiles',
];

const nodeConfigSnapshots = new Map<string, RuntimeConfig>();
let pendingApplyNodeIDs = new Set<string>();

export function nodeIDOf(value: object | null | undefined): string {
  return (value as NodeOwned | null | undefined)?.ManagedNodeID || LOCAL_NODE_ID;
}

export function nodeObjectKey(value: ManagedConfigObject): string {
  return `${nodeIDOf(value)}:${value.ID}`;
}

export function sameNodeObject(left: ManagedConfigObject, right: ManagedConfigObject): boolean {
  return left.ID === right.ID && nodeIDOf(left) === nodeIDOf(right);
}

export function defaultTargetNodeID(scope: string): string {
  return scope && scope !== ALL_NODE_ID ? scope : LOCAL_NODE_ID;
}

export function filterNodeOwned<T extends NodeOwned>(values: T[], scope: string): T[] {
  return scope === ALL_NODE_ID ? values : values.filter((value) => nodeIDOf(value) === scope);
}

export function filterConfigByNode(config: RuntimeConfig, scope: string): RuntimeConfig {
  if (scope === ALL_NODE_ID) return config;
  const next: RuntimeConfig = { ...config };
  for (const key of configListKeys) {
    const values = (config[key] || []) as Array<NodeOwned>;
    (next as Record<string, unknown>)[key] = filterNodeOwned(values, scope);
  }
  return next;
}

export async function getManagedRuntimeConfig(): Promise<RuntimeConfig> {
  const localConfig = tagConfig(await getRuntimeConfig(), LOCAL_NODE_ID);
  nodeConfigSnapshots.clear();
  nodeConfigSnapshots.set(LOCAL_NODE_ID, stripNodeMetadata(localConfig));
  const nodes = (await loadManagedNodes()).filter((node) => node.Enabled);
  const results = await Promise.allSettled(nodes.map(async (node) => ({
    node,
    config: await getManagedNodeConfig(node.ID),
  })));
  const remoteConfigs: RuntimeConfig[] = [];
  for (const result of results) {
    if (result.status !== 'fulfilled') continue;
    nodeConfigSnapshots.set(result.value.node.ID, result.value.config);
    remoteConfigs.push(tagConfig(result.value.config, result.value.node.ID));
  }
  return mergeConfigs([localConfig, ...remoteConfigs]);
}

export async function getManagedStats(scope: string = ALL_NODE_ID): Promise<ManagedStatsReport> {
  let nodeIDs: string[];
  if (scope === ALL_NODE_ID) {
    const nodes = (await loadManagedNodes()).filter((node) => node.Enabled);
    nodeIDs = [LOCAL_NODE_ID, ...nodes.map((node) => node.ID)];
  } else {
    nodeIDs = [scope || LOCAL_NODE_ID];
  }
  const results = await Promise.allSettled(nodeIDs.map(async (nodeID) => ({
    nodeID,
    report: await getStats(nodeID),
  })));
  const reports = results.flatMap((result) => result.status === 'fulfilled' ? [result.value] : []);
  if (reports.length === 0) {
    const failure = results.find((result): result is PromiseRejectedResult => result.status === 'rejected');
    throw failure?.reason instanceof Error ? failure.reason : new Error('No node statistics are available.');
  }
  return mergeManagedStats(reports);
}

export function mergeManagedStats(reports: Array<{ nodeID: string; report: StatsReport }>): ManagedStatsReport {
  const tagBuckets = (nodeID: string, values: StatsBucket[] | undefined): ManagedStatsBucket[] => (
    (values || []).map((value) => ({ ...value, ManagedNodeID: nodeID }))
  );
  const tagClients = (nodeID: string, values: ClientQuotaState[] | undefined): ManagedClientQuotaState[] => (
    (values || []).map((value) => ({ ...value, ManagedNodeID: nodeID }))
  );
  return {
    generatedAt: reports.map((item) => item.report.generatedAt).filter(Boolean).sort().at(-1),
    totals: reports.reduce((total, item) => addStatsCounters(total, item.report.totals), {} as StatsCounters),
    byEndpoint: reports.flatMap((item) => tagBuckets(item.nodeID, item.report.byEndpoint)),
    byClient: reports.flatMap((item) => tagBuckets(item.nodeID, item.report.byClient)),
    clients: reports.flatMap((item) => tagClients(item.nodeID, item.report.clients)),
  };
}

export async function saveManagedRuntimeConfig(config: RuntimeConfig): Promise<RuntimeConfig> {
  const ownedConfig = propagateManagedNodeOwnership(config);
  const nodes = (await loadManagedNodes()).filter((node) => node.Enabled);
  const orderedNodeIDs = [LOCAL_NODE_ID, ...nodes.map((node) => node.ID)];
  const targets = new Map<string, RuntimeConfig>();
  for (const nodeID of orderedNodeIDs) {
    const target = stripNodeMetadata(configForNode(ownedConfig, nodeID));
    if (nodeID !== LOCAL_NODE_ID) target.Settings = nodeConfigSnapshots.get(nodeID)?.Settings || [];
    targets.set(nodeID, target);
  }

  const changedNodeIDs = orderedNodeIDs.filter((nodeID) => {
    const previous = nodeConfigSnapshots.get(nodeID);
    const target = targets.get(nodeID);
    return target !== undefined && (!previous || configDigest(previous) !== configDigest(target));
  });
  for (const nodeID of changedNodeIDs) {
    if (nodeID !== LOCAL_NODE_ID && !nodeConfigSnapshots.has(nodeID)) {
      throw new Error(`Node ${nodeID} configuration is unavailable; reconnect before editing it.`);
    }
  }

  const completed: string[] = [];
  const previousConfigs = new Map(nodeConfigSnapshots);
  try {
    for (const nodeID of changedNodeIDs) {
      const target = targets.get(nodeID) || {};
      const saved = nodeID === LOCAL_NODE_ID
        ? await saveRuntimeConfig(target)
        : await saveManagedNodeConfig(nodeID, target);
      nodeConfigSnapshots.set(nodeID, saved);
      completed.push(nodeID);
    }
  } catch (error) {
    await rollbackNodeConfigs(completed, previousConfigs);
    nodeConfigSnapshots.clear();
    for (const [nodeID, previous] of previousConfigs) nodeConfigSnapshots.set(nodeID, previous);
    throw error;
  }

  pendingApplyNodeIDs = new Set(changedNodeIDs);
  return mergeConfigs(orderedNodeIDs
    .filter((nodeID) => nodeConfigSnapshots.has(nodeID))
    .map((nodeID) => tagConfig(nodeConfigSnapshots.get(nodeID) || {}, nodeID)));
}

export function propagateManagedNodeOwnership(config: RuntimeConfig): RuntimeConfig {
  const next = cloneConfigLists(config);
  const devices = records(next.Devices);
  const listeners = records(next.Listeners);
  const connectors = records(next.Connectors);
  const clients = records(next.Clients);
  const routes = records(next.Routes);
  const vkeys = records(next.VKeys);
  const addresses = records(next.Addresses);
  const profiles = records(next.XrayProfiles);
  const endpoints = [...listeners, ...connectors];

  const explicitOwnerOf = (value: Record<string, unknown> | undefined): string | undefined => (
    typeof value?.ManagedNodeID === 'string' ? value.ManagedNodeID : undefined
  );
  const ownerOf = (
    values: Array<Record<string, unknown>>,
    id: unknown,
    preferredOwner?: string,
  ): string | undefined => {
    if (!id) return undefined;
    const matches = values.filter((item) => item.ID === id);
    if (preferredOwner && matches.some((item) => explicitOwnerOf(item) === preferredOwner)) return preferredOwner;
    const owners = [...new Set(matches.map(explicitOwnerOf).filter((owner): owner is string => Boolean(owner)))];
    return owners.length === 1 ? owners[0] : undefined;
  };
  const setOwner = (value: Record<string, unknown> | undefined, owner: string | undefined): boolean => {
    if (!value || !owner || typeof value.ManagedNodeID === 'string') return false;
    value.ManagedNodeID = owner;
    return true;
  };
  const setReferencedOwner = (
    values: Array<Record<string, unknown>>,
    id: unknown,
    owner: string | undefined,
  ): boolean => {
    if (!id || !owner) return false;
    if (values.some((item) => item.ID === id && explicitOwnerOf(item) === owner)) return false;
    const unowned = values.filter((item) => item.ID === id && !explicitOwnerOf(item));
    return unowned.length === 1 ? setOwner(unowned[0], owner) : false;
  };
  const bindingOf = (value: Record<string, unknown>): Record<string, unknown> => (
    value.Binding && typeof value.Binding === 'object' ? value.Binding as Record<string, unknown> : {}
  );
  const idsOf = (value: unknown): unknown[] => Array.isArray(value) ? value : value ? [value] : [];

  // Generated devices, credentials, profiles and limits inherit the node of the
  // endpoint/user/route that owns their reference. Repeat because a route can
  // connect objects whose ownership was inferred in the preceding pass.
  for (let pass = 0; pass < 4; pass += 1) {
    let changed = false;
    for (const client of clients) {
      const currentOwner = explicitOwnerOf(client);
      const listenerIDs = [...idsOf(client.ListenerIDs), ...idsOf(client.ListenerID)];
      const owner = listenerIDs.map((id) => ownerOf(listeners, id, currentOwner)).find(Boolean);
      changed = setOwner(client, owner) || changed;
    }
    for (const endpoint of endpoints) {
      const owner = explicitOwnerOf(endpoint);
      const binding = bindingOf(endpoint);
      changed = setReferencedOwner(devices, binding.DeviceID, owner) || changed;
      changed = setReferencedOwner(vkeys, binding.VKeyID, owner) || changed;
      changed = setReferencedOwner(addresses, binding.AddressID, owner) || changed;
      changed = setReferencedOwner(profiles, endpoint.XrayProfileID, owner) || changed;
    }
    for (const client of clients) {
      const owner = explicitOwnerOf(client);
      const binding = bindingOf(client);
      changed = setReferencedOwner(vkeys, binding.VKeyID, owner) || changed;
      changed = setReferencedOwner(addresses, client.AddressID || binding.AddressID, owner) || changed;
    }
    for (const route of routes) {
      const currentOwner = explicitOwnerOf(route);
      const inferred = [
        ownerOf(listeners, route.ListenerID, currentOwner),
        ownerOf(connectors, route.ConnectorID, currentOwner),
        ownerOf(clients, route.ClientID, currentOwner),
        ownerOf(devices, route.DeviceID, currentOwner),
        ownerOf(vkeys, route.VKeyID, currentOwner),
        ownerOf(addresses, route.AddressID, currentOwner),
      ].find(Boolean);
      changed = setOwner(route, inferred) || changed;
      const owner = explicitOwnerOf(route);
      changed = setReferencedOwner(devices, route.DeviceID, owner) || changed;
      changed = setReferencedOwner(vkeys, route.VKeyID, owner) || changed;
      changed = setReferencedOwner(addresses, route.AddressID, owner) || changed;
    }
    if (!changed) break;
  }
  return next;
}

export async function applyManagedRuntimeConfig(): Promise<void> {
  const nodeIDs = [...pendingApplyNodeIDs];
  for (const nodeID of nodeIDs) {
    if (nodeID === LOCAL_NODE_ID) await applyRuntimeConfig();
    else await applyManagedNodeConfig(nodeID);
  }
  pendingApplyNodeIDs.clear();
}

function configForNode(config: RuntimeConfig, nodeID: string): RuntimeConfig {
  const next: RuntimeConfig = {};
  for (const key of configListKeys) {
    const values = (config[key] || []) as Array<NodeOwned>;
    (next as Record<string, unknown>)[key] = values.filter((value) => nodeIDOf(value) === nodeID);
  }
  if (nodeID === LOCAL_NODE_ID) next.Settings = config.Settings;
  return next;
}

function mergeConfigs(configs: RuntimeConfig[]): RuntimeConfig {
  const merged: RuntimeConfig = {};
  for (const key of configListKeys) {
    (merged as Record<string, unknown>)[key] = configs.flatMap((config) => (config[key] || []) as unknown[]);
  }
  merged.Settings = configs[0]?.Settings || [];
  return merged;
}

function cloneConfigLists(config: RuntimeConfig): RuntimeConfig {
  const next: RuntimeConfig = { ...config };
  for (const key of configListKeys) {
    const values = (config[key] || []) as unknown as Array<Record<string, unknown>>;
    (next as Record<string, unknown>)[key] = values.map((value) => ({ ...value }));
  }
  return next;
}

function records(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value as Array<Record<string, unknown>> : [];
}

function tagConfig(config: RuntimeConfig, nodeID: string): RuntimeConfig {
  const next: RuntimeConfig = { ...config };
  for (const key of configListKeys) {
    const values = (config[key] || []) as unknown as Array<Record<string, unknown>>;
    (next as Record<string, unknown>)[key] = values.map((value) => ({ ...value, ManagedNodeID: nodeID }));
  }
  return next;
}

function stripNodeMetadata(config: RuntimeConfig): RuntimeConfig {
  const next: RuntimeConfig = { ...config };
  for (const key of configListKeys) {
    const values = (config[key] || []) as unknown as Array<Record<string, unknown>>;
    (next as Record<string, unknown>)[key] = values.map(({ ManagedNodeID: _managedNodeID, ...value }) => value);
  }
  return next;
}

async function rollbackNodeConfigs(nodeIDs: string[], previousConfigs: Map<string, RuntimeConfig>): Promise<void> {
  for (const nodeID of [...nodeIDs].reverse()) {
    const previous = previousConfigs.get(nodeID);
    if (!previous) continue;
    try {
      if (nodeID === LOCAL_NODE_ID) await saveRuntimeConfig(previous);
      else await saveManagedNodeConfig(nodeID, previous);
    } catch {
      // Keep the original save error. A later reload reconciles partial state.
    }
  }
}

function configDigest(config: RuntimeConfig): string {
  return JSON.stringify(config);
}

function addStatsCounters(left: StatsCounters, right?: StatsCounters): StatsCounters {
  return {
    rxPackets: Number(left.rxPackets || 0) + Number(right?.rxPackets || 0),
    txPackets: Number(left.txPackets || 0) + Number(right?.txPackets || 0),
    rxBytes: Number(left.rxBytes || 0) + Number(right?.rxBytes || 0),
    txBytes: Number(left.txBytes || 0) + Number(right?.txBytes || 0),
    dropsGuard: Number(left.dropsGuard || 0) + Number(right?.dropsGuard || 0),
    dropsIO: Number(left.dropsIO || 0) + Number(right?.dropsIO || 0),
  };
}
