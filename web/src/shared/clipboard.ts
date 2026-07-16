export async function copyText(value: string): Promise<void> {
  if (copyWithBrowserEvent(value)) return;

  const clipboard = typeof navigator !== 'undefined' ? navigator.clipboard : undefined;
  if (!clipboard?.writeText) throw new Error('clipboard is unavailable');
  await clipboard.writeText(value);
}

function copyWithBrowserEvent(value: string): boolean {
  if (
    typeof document === 'undefined'
    || typeof document.addEventListener !== 'function'
    || typeof document.removeEventListener !== 'function'
    || typeof document.execCommand !== 'function'
  ) return false;

  let copied = false;
  const handleCopy = (event: ClipboardEvent) => {
    if (!event.clipboardData) return;
    event.preventDefault();
    event.clipboardData.clearData();
    event.clipboardData.setData('text/plain', value);
    copied = true;
  };

  document.addEventListener('copy', handleCopy, true);
  try {
    document.execCommand('copy');
    return copied;
  } catch {
    return false;
  } finally {
    document.removeEventListener('copy', handleCopy, true);
  }
}
