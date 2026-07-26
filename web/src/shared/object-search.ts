export function objectMatchesSearch(value: unknown, query: string, extraValues: unknown[] = []): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) return true;
  const serialized = [value, ...extraValues]
    .map((item) => typeof item === 'string' ? item : JSON.stringify(item ?? ''))
    .join(' ')
    .toLocaleLowerCase();
  return serialized.includes(needle);
}
