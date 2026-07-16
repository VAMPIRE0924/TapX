export function hashFromPath(path: string): string {
  const index = path.indexOf('#');
  return index >= 0 ? path.slice(index) : '';
}
