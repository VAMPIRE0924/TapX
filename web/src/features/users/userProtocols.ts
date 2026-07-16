import type { TapxClient, TapxEndpoint, TapxXrayProfile } from '../../shared/api';

type NodeOwned = { ManagedNodeID?: string };

function nodeIDOf(value: NodeOwned | undefined): string {
  return value?.ManagedNodeID || 'local';
}

export function userProtocols(
  user: TapxClient,
  listeners: TapxEndpoint[],
  profiles: TapxXrayProfile[],
): string[] {
  const listenerIDs = Array.isArray(user.ListenerIDs)
    ? user.ListenerIDs.filter(Boolean)
    : user.ListenerID
      ? [user.ListenerID]
      : [];
  const ownerID = nodeIDOf(user as NodeOwned);
  const listenerByID = new Map(listeners
    .filter((listener) => nodeIDOf(listener as NodeOwned) === ownerID)
    .map((listener) => [listener.ID, listener]));
  const profileByID = new Map(profiles
    .filter((profile) => nodeIDOf(profile as NodeOwned) === ownerID)
    .map((profile) => [profile.ID, profile]));
  const protocols = listenerIDs
    .map((id) => listenerProtocol(listenerByID.get(id), profileByID))
    .filter((protocol): protocol is string => Boolean(protocol));
  if (protocols.length === 0 && user.CredentialType) protocols.push(user.CredentialType);
  return [...new Set(protocols)];
}

function listenerProtocol(
  listener: TapxEndpoint | undefined,
  profileByID: Map<string, TapxXrayProfile>,
): string | undefined {
  if (!listener) return undefined;
  if (listener.Transport === 'udp') return 'raw-udp';
  if (listener.Transport === 'tcp') return 'raw-tcp';
  if (listener.XrayProfileID) return profileByID.get(listener.XrayProfileID)?.InboundProtocol || undefined;
  return undefined;
}
