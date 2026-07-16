export function numberValue(value: unknown, fallback?: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : (fallback || 0);
}

export function stringValue(value: unknown, fallback?: string): string {
  return typeof value === 'string' ? value : (fallback || '');
}

export function booleanValue(value: unknown, fallback?: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback === true;
}
