export function stripTapxSocketOverrides<T extends Record<string, unknown>>(input: T): T {
  const output: Record<string, unknown> = { ...input };
  for (const key of [
    'PeerMode',
    'FixedPeer',
    'BindInterface',
    'BindAddress',
    'ReceiveBuffer',
    'SendBuffer',
    'ReuseAddr',
    'ReusePort',
    'ReadBuffer',
    'WriteBuffer',
  ]) {
    delete output[key];
  }
  return output as T;
}
