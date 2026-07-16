import { randomLowerAndNumber, randomShortIds } from '../../../shared/random';

export function newInboundCertificate() {
  return {
    useFile: true,
    certificateFile: '',
    keyFile: '',
    certificate: [],
    key: [],
    ocspStapling: 0,
    oneTimeLoading: false,
    usage: 'encipherment',
    buildChain: false,
  };
}

export function newInboundTlsSettings(hysteria = false) {
  return {
    serverName: '',
    minVersion: '1.2',
    maxVersion: '1.3',
    cipherSuites: '',
    rejectUnknownSni: false,
    disableSystemRoot: false,
    enableSessionResumption: false,
    certificates: [newInboundCertificate()],
    alpn: hysteria ? ['h3'] : ['h2', 'http/1.1'],
    echServerKeys: '',
    settings: {
      fingerprint: hysteria ? '' : 'chrome',
      echConfigList: '',
      pinnedPeerCertSha256: [],
      verifyPeerCertByName: '',
    },
  };
}

export function newOutboundTlsSettings() {
  return {
    serverName: '',
    alpn: [],
    fingerprint: '',
    echConfigList: '',
    verifyPeerCertByName: '',
    pinnedPeerCertSha256: '',
  };
}

export function newInboundRealitySettings() {
  return {
    show: false,
    xver: 0,
    target: '',
    serverNames: [],
    privateKey: '',
    minClientVer: '',
    maxClientVer: '',
    maxTimediff: 0,
    shortIds: randomShortIds(),
    mldsa65Seed: '',
    settings: {
      publicKey: '',
      fingerprint: 'chrome',
      serverName: '',
      spiderX: `/${randomLowerAndNumber(15)}`,
      mldsa65Verify: '',
    },
  };
}

export function newOutboundRealitySettings() {
  return {
    publicKey: '',
    fingerprint: 'chrome',
    serverName: '',
    shortId: '',
    spiderX: '',
    mldsa65Verify: '',
  };
}
