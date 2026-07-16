export function unixSeconds(date = Date.now()): number {
  return Math.floor(date / 1000);
}
