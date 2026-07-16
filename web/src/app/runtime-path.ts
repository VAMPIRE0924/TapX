declare global {
  interface Window {
    __TAPX_BASE_PATH__?: string;
  }
}

export function normalizeBasePath(value: string | undefined): string {
  const trimmed = (value || '').trim();
  if (!trimmed || trimmed === '/') return '';
  const prefixed = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return prefixed.replace(/\/+$/, '');
}

export function panelBasePath(): string {
  if (typeof window !== 'undefined' && window.__TAPX_BASE_PATH__ !== undefined) {
    return normalizeBasePath(window.__TAPX_BASE_PATH__);
  }
  if (typeof document === 'undefined') return '';
  const declared = document.querySelector<HTMLMetaElement>('meta[name="tapx-base-path"]')?.content;
  if (declared !== undefined) return normalizeBasePath(declared);
  const baseElement = document.querySelector<HTMLBaseElement>('base');
  if (!baseElement) return '';
  try {
    return normalizeBasePath(new URL(baseElement.href).pathname);
  } catch {
    return '';
  }
}

export function appPathname(pathname?: string, basePath = panelBasePath()): string {
  const current = pathname ?? (typeof window === 'undefined' ? '/' : window.location.pathname);
  const base = normalizeBasePath(basePath);
  if (!base) return current || '/';
  if (current === base) return '/';
  if (current.startsWith(`${base}/`)) return current.slice(base.length) || '/';
  return current || '/';
}

export function appLocation(): string {
  if (typeof window === 'undefined') return '/';
  return `${appPathname()}${window.location.search}${window.location.hash}`;
}

export function panelPath(path: string, basePath = panelBasePath()): string {
  if (!path.startsWith('/')) return path;
  return `${normalizeBasePath(basePath)}${path}` || '/';
}

export function panelFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  if (typeof input === 'string' && input.startsWith('/')) return fetch(panelPath(input), init);
  return fetch(input, init);
}
