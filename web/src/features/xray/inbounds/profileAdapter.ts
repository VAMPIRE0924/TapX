type JsonObject = Record<string, unknown>;

export function inboundSettingsToWire(protocol: string, input: JsonObject): JsonObject {
  const output = cloneObject(input);
  if (protocol === 'wireguard') delete output.pubKey;
  return pruneUndefined(output) as JsonObject;
}

export function inboundSettingsFromWire(protocol: string, input: JsonObject): JsonObject {
  const output = cloneObject(input);
  void protocol;
  return output;
}

export function takeInboundFallbacks(settings: JsonObject): { settings: JsonObject; fallbacks?: unknown[] } {
  const output = cloneObject(settings);
  const fallbacks = Array.isArray(output.fallbacks) ? [...output.fallbacks] : undefined;
  delete output.fallbacks;
  return { settings: output, fallbacks };
}

export function restoreInboundFallbacks(settings: JsonObject, fallbacks: unknown): JsonObject {
  if (!Array.isArray(fallbacks)) return settings;
  return { ...settings, fallbacks };
}

export function inboundStreamToWire(input: JsonObject): JsonObject {
  const output = cloneObject(input);
  const xhttp = objectValue(output.xhttpSettings);
  if (Object.keys(xhttp).length > 0) {
    const xmuxEnabled = xhttp.enableXmux === true;
    delete xhttp.enableXmux;
    delete xhttp.uplinkChunkSize;
    if (!xmuxEnabled) delete xhttp.xmux;
    output.xhttpSettings = xhttp;
  }
  stripCertificateViewFields(output);
  return pruneUndefined(output) as JsonObject;
}

export function inboundStreamFromWire(input: JsonObject): JsonObject {
  const output = cloneObject(input);
  const xhttp = objectValue(output.xhttpSettings);
  if (Object.keys(xhttp).length > 0) {
    xhttp.enableXmux = Object.keys(objectValue(xhttp.xmux)).length > 0;
    output.xhttpSettings = xhttp;
  }
  synthesizeCertificateViewFields(output);
  return output;
}

function stripCertificateViewFields(stream: JsonObject) {
  const tls = objectValue(stream.tlsSettings);
  if (!Array.isArray(tls.certificates)) return;
  tls.certificates = tls.certificates.map((item) => {
    const certificate = objectValue(item);
    delete certificate.useFile;
    return certificate;
  });
  stream.tlsSettings = tls;
}

function synthesizeCertificateViewFields(stream: JsonObject) {
  const tls = objectValue(stream.tlsSettings);
  if (!Array.isArray(tls.certificates)) return;
  tls.certificates = tls.certificates.map((item) => {
    const certificate = objectValue(item);
    return {
      ...certificate,
      useFile: Boolean(certificate.certificateFile || certificate.keyFile || (!certificate.certificate && !certificate.key)),
    };
  });
  stream.tlsSettings = tls;
}

function cloneObject(value: unknown): JsonObject {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  return JSON.parse(JSON.stringify(value)) as JsonObject;
}

function objectValue(value: unknown): JsonObject {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as JsonObject : {};
}

function pruneUndefined(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(pruneUndefined);
  if (value && typeof value === 'object') {
    const output: JsonObject = {};
    for (const [key, item] of Object.entries(value as JsonObject)) {
      if (item !== undefined) output[key] = pruneUndefined(item);
    }
    return output;
  }
  return value;
}
