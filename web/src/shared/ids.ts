export function safeID(value: string): string {
  const cleaned = value.trim().replace(/[^a-zA-Z0-9_.-]/g, '-').replace(/-+/g, '-');
  return cleaned || 'item';
}

export function uniqueID(base: string, used: Set<string>): string {
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}
