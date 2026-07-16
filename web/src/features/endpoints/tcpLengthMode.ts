export type TcpLengthMode = 'uint16' | 'uint32';

export function resolveTcpLengthMode({
  mode,
  legacyPrefix,
  stored,
}: {
  mode?: unknown;
  legacyPrefix?: unknown;
  stored?: unknown;
}): TcpLengthMode {
  if (mode === 'uint16' || mode === 'uint32') return mode;
  if (legacyPrefix === true) return 'uint32';
  if (legacyPrefix === false) return 'uint16';
  if (stored === 'uint16' || stored === 'uint32') return stored;
  return 'uint16';
}
