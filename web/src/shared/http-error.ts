const maximumBodyLength = 500;

export async function responseError(response: Response, fallback: string): Promise<Error> {
  const body = (await response.text()).trim();
  const message = jsonErrorMessage(body) || plainErrorMessage(body) || `${fallback} (${response.status})`;
  return new Error(message);
}

function jsonErrorMessage(body: string): string {
  if (!body.startsWith('{')) return '';
  try {
    const payload = JSON.parse(body) as Record<string, unknown>;
    for (const key of ['error', 'message', 'msg']) {
      const value = payload[key];
      if (typeof value === 'string' && value.trim()) return value.trim().slice(0, maximumBodyLength);
    }
  } catch {
    return '';
  }
  return '';
}

function plainErrorMessage(body: string): string {
  if (!body || /<\/?(?:html|body|head|script)\b/i.test(body)) return '';
  return body.slice(0, maximumBodyLength);
}
