export interface RuntimePipeSummary {
  inactive?: boolean;
  xrayRuntime?: string;
}

export interface RuntimePipeCollection {
  running?: boolean;
  componentStates?: {
    tapx?: boolean;
  };
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

export function tapxComponentRunning(runtime?: RuntimePipeCollection): boolean {
  if (typeof runtime?.componentStates?.tapx === 'boolean') {
    return runtime.componentStates.tapx;
  }
  return activeTapXPipeCount(runtime) > 0 || runtime?.running === true;
}
