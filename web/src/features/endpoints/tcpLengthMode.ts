export type TcpLengthMode = 'uint16' | 'uint32';

export function resolveTcpLengthMode({
  mode,
}: {
  mode?: unknown;
}): TcpLengthMode {
  if (mode === 'uint16' || mode === 'uint32') return mode;
  return 'uint16';
}
