export const defaultPanelName = 'TapX-UI';

export function panelDisplayName(value: unknown): string {
  return typeof value === 'string' && value.trim() ? value.trim() : defaultPanelName;
}

export function applyPanelTitle(value: unknown): void {
  document.title = panelDisplayName(value);
}
