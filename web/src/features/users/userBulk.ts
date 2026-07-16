export type BulkNameInput = {
  method: number;
  first: number;
  last: number;
  prefix: string;
  postfix: string;
  quantity: number;
};

export function buildBulkNames(input: BulkNameInput, randomName: () => string): string[] {
  const start = input.method > 1 ? Math.max(1, Math.trunc(input.first || 1)) : 0;
  const end = input.method > 1
    ? Math.max(start, Math.trunc(input.last || start)) + 1
    : Math.max(1, Math.trunc(input.quantity || 1));
  const output: string[] = [];
  for (let index = start; index < end; index += 1) {
    const random = input.method === 4 ? '' : randomName();
    const sequence = input.method > 1 ? String(index) : '';
    output.push(`${random}${input.method > 0 ? input.prefix || '' : ''}${sequence}${input.method > 2 ? input.postfix || '' : ''}`);
  }
  return output;
}

export function parseBulkExpiry(value: string): number | null {
  const input = String(value || '').trim();
  if (!input) return 0;
  const parsed = Date.parse(input.replace(' ', 'T'));
  return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : null;
}

export function changeListenerIDs(current: string[], targets: string[], mode: 'attach' | 'detach'): string[] {
  if (mode === 'attach') return [...new Set([...current, ...targets].filter(Boolean))];
  const removed = new Set(targets);
  return current.filter((id) => id && !removed.has(id));
}

export type BulkUserAdjustment = {
  addDays: number;
  addTrafficGB: number;
  uploadRateMbps?: number | null;
  downloadRateMbps?: number | null;
};

type AdjustableUser = {
  ExpiresAt?: number;
  TrafficCap?: number;
  UploadRateLimit?: number;
  DownloadRateLimit?: number;
  UpdatedAt?: number;
};

export function applyBulkUserAdjustment<T extends AdjustableUser>(
  user: T,
  input: BulkUserAdjustment,
  now: number,
): T {
  const addSeconds = Math.trunc(input.addDays || 0) * 86400;
  const addBytes = Math.trunc((input.addTrafficGB || 0) * 1024 * 1024 * 1024);
  let expiresAt = user.ExpiresAt || 0;
  if (addSeconds !== 0) expiresAt = Math.max(0, (expiresAt > 0 ? expiresAt : now) + addSeconds);
  const next: AdjustableUser = {
    ...user,
    ExpiresAt: expiresAt,
    TrafficCap: Math.max(0, (user.TrafficCap || 0) + addBytes),
    UpdatedAt: now,
  };
  if (typeof input.uploadRateMbps === 'number') next.UploadRateLimit = mbpsToBps(input.uploadRateMbps);
  if (typeof input.downloadRateMbps === 'number') next.DownloadRateLimit = mbpsToBps(input.downloadRateMbps);
  return next as T;
}

function mbpsToBps(value: number): number {
  return value > 0 ? Math.round(value * 1_000_000) : 0;
}
