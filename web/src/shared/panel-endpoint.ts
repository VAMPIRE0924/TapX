export type PanelEndpointValues = {
  listenDomain?: string;
  listenPort?: number;
  uriPath?: string;
  panelHTTPS?: boolean;
  certPublicPath?: string;
  certPrivatePath?: string;
};

export type BrowserLocation = Pick<Location, 'protocol' | 'hostname'>;

export function panelRestartURL(values: PanelEndpointValues, location: BrowserLocation): string {
  const https = values.panelHTTPS === true || Boolean(values.certPublicPath?.trim() && values.certPrivatePath?.trim());
  const protocol = https ? 'https:' : 'http:';
  const host = values.listenDomain?.trim() || location.hostname;
  const hostname = host.includes(':') && !host.startsWith('[') ? `[${host}]` : host;
  const port = Number(values.listenPort) || (https ? 443 : 80);
  const portPart = (https && port === 443) || (!https && port === 80) ? '' : `:${port}`;
  return `${protocol}//${hostname}${portPart}${normalizePanelPath(values.uriPath)}`;
}

function normalizePanelPath(value?: string): string {
  const trimmed = value?.trim() || '/';
  const leading = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return leading.endsWith('/') ? leading : `${leading}/`;
}
