import type { TapxEndpoint, TapxXrayProfile } from './api';

type NodeOwned = { ManagedNodeID?: string };

function nodeIDOf(value: NodeOwned | undefined): string {
  return value?.ManagedNodeID || 'local';
}

export function upsertXrayProfile(profiles: TapxXrayProfile[], profile: TapxXrayProfile): TapxXrayProfile[] {
  const ownerID = nodeIDOf(profile as NodeOwned);
  const index = profiles.findIndex((item) => item.ID === profile.ID && nodeIDOf(item as NodeOwned) === ownerID);
  if (index < 0) return [...profiles, profile];
  return profiles.map((item, itemIndex) => (itemIndex === index ? profile : item));
}

export function removeUnusedXrayProfiles(
  profiles: TapxXrayProfile[],
  candidateIDs: Array<string | undefined>,
  references: { listeners: TapxEndpoint[]; connectors: TapxEndpoint[] },
): TapxXrayProfile[] {
  const candidates = new Set(candidateIDs.filter((id): id is string => Boolean(id)));
  if (candidates.size === 0) return profiles;

  const used = new Set<string>();
  for (const endpoint of [...references.listeners, ...references.connectors]) {
    if (endpoint.XrayProfileID) used.add(`${nodeIDOf(endpoint as NodeOwned)}:${endpoint.XrayProfileID}`);
  }
  return profiles.filter((profile) => !candidates.has(profile.ID) || used.has(`${nodeIDOf(profile as NodeOwned)}:${profile.ID}`));
}
