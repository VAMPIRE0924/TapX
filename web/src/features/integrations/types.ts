export interface WireguardIntegrationDraft {
  tag: string;
  settings: {
    mtu?: number;
    secretKey?: string;
    address?: string[];
    reserved?: number[];
    domainStrategy?: string;
    peers: Array<{ publicKey?: string; endpoint?: string }>;
    noKernelTun: boolean;
  };
}
