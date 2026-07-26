export interface RuntimePipeSummary {
  inactive?: boolean;
  xrayRuntime?: string;
}

export interface RuntimePipeCollection {
  udpPipes?: unknown[];
  tcpPipes?: unknown[];
}

export function activeRawTCPPipes(runtime?: RuntimePipeCollection): RuntimePipeSummary[] {
  return (runtime?.tcpPipes || [])
    .map((item) => item as RuntimePipeSummary)
    .filter((item) => !item.inactive && !item.xrayRuntime);
}

export function activeExternalXrayPipes(runtime?: RuntimePipeCollection): RuntimePipeSummary[] {
  return (runtime?.tcpPipes || [])
    .map((item) => item as RuntimePipeSummary)
    .filter((item) => !item.inactive && item.xrayRuntime === 'external');
}

export function activeTapXPipeCount(runtime?: RuntimePipeCollection): number {
  return (runtime?.udpPipes?.length || 0) + activeRawTCPPipes(runtime).length;
}
