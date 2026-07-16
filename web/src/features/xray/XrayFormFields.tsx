import { Form, Radio, Select, type FormInstance } from 'antd';
import { BlackholeOutboundFields, LoopbackOutboundFields } from './outbounds/BlackholeLoopbackOutboundFields';
import { DnsOutboundFields } from './outbounds/DnsOutboundFields';
import { FreedomOutboundFields } from './outbounds/FreedomOutboundFields';
import { HttpOutboundFields, SocksOutboundFields } from './outbounds/HttpSocksOutboundFields';
import { ServerTargetFields } from './outbounds/ServerTargetFields';
import { ShadowsocksOutboundFields } from './outbounds/ShadowsocksOutboundFields';
import { TrojanOutboundFields } from './outbounds/TrojanOutboundFields';
import { VlessOutboundFields } from './outbounds/VlessOutboundFields';
import { VmessOutboundFields } from './outbounds/VmessOutboundFields';
import { WireguardOutboundFields } from './outbounds/WireguardOutboundFields';
import { HysteriaFields } from './protocols/HysteriaFields';
import { HttpInboundFields, MixedInboundFields } from './protocols/HttpMixedInboundFields';
import { MtprotoInboundFields } from './protocols/MtprotoInboundFields';
import { ShadowsocksInboundFields } from './protocols/ShadowsocksInboundFields';
import { TunnelInboundFields } from './protocols/TunnelInboundFields';
import { TunInboundFields } from './protocols/TunInboundFields';
import { VlessInboundFields } from './protocols/VlessInboundFields';
import { WireguardInboundFields } from './protocols/WireguardInboundFields';
import { XrayInboundRealityFields, XrayOutboundRealityFields } from './security/RealityFields';
import { getPanelObject } from './security/api';
import { XrayInboundTlsFields, XrayOutboundTlsFields } from './security/TlsFields';
import { SniffingFields } from './shared/SniffingFields';
import { FinalMaskFields } from './transport/FinalMaskFields';
import { randomText } from '../../shared/random';
import {
  newInboundRealitySettings,
  newInboundTlsSettings,
  newOutboundRealitySettings,
  newOutboundTlsSettings,
} from './security/defaults';
import { MuxFields } from './transport/MuxFields';
import { RawTransportFields } from './transport/RawTransportFields';
import { SockoptFields } from './transport/SockoptFields';
import { GrpcTransportFields, HttpUpgradeTransportFields, KcpTransportFields, WsTransportFields } from './transport/StandardTransportFields';
import { XhttpTransportFields } from './transport/XhttpTransportFields';
import { useI18n } from '../../i18n/I18nProvider';

export type XrayDirection = 'inbound' | 'outbound';

export const inboundXrayProtocolOptions = [
  'vmess',
  'vless',
  'trojan',
  'shadowsocks',
  'wireguard',
  'hysteria',
  'http',
  'mixed',
  'tunnel',
  'tun',
  'mtproto',
].map((value) => ({ value, label: value }));

export const outboundXrayProtocolOptions = [
  'freedom',
  'blackhole',
  'dns',
  'vmess',
  'vless',
  'trojan',
  'shadowsocks',
  'wireguard',
  'hysteria',
  'socks',
  'http',
  'loopback',
].map((value) => ({ value, label: value }));

export const networkOptions = [
  { value: 'tcp', label: 'RAW' },
  { value: 'kcp', label: 'mKCP' },
  { value: 'ws', label: 'WebSocket' },
  { value: 'grpc', label: 'gRPC' },
  { value: 'httpupgrade', label: 'HTTPUpgrade' },
  { value: 'xhttp', label: 'XHTTP' },
];

export { alpnOptions, targetStrategyOptions } from './options';

const tlsProtocols = new Set(['vmess', 'vless', 'trojan', 'shadowsocks']);
const tlsNetworks = new Set(['tcp', 'ws', 'http', 'grpc', 'httpupgrade', 'xhttp']);
const realityProtocols = new Set(['vless', 'trojan']);
const realityNetworks = new Set(['tcp', 'http', 'grpc', 'xhttp']);
const streamProtocols = new Set(['vmess', 'vless', 'trojan', 'shadowsocks', 'hysteria', 'wireguard', 'tunnel']);
const serverProtocols = new Set(['vmess', 'vless', 'trojan', 'shadowsocks', 'socks', 'http', 'hysteria']);
const muxProtocols = new Set(['vmess', 'vless', 'trojan', 'shadowsocks', 'http', 'socks']);

export function canEnableStream(protocol: string): boolean {
  return streamProtocols.has(protocol);
}

export function canEnableTls(protocol: string, network: string): boolean {
  if (protocol === 'hysteria') return true;
  return tlsProtocols.has(protocol) && tlsNetworks.has(network);
}

export function canEnableReality(protocol: string, network: string): boolean {
  return realityProtocols.has(protocol) && realityNetworks.has(network);
}

export function canEnableTlsFlow(protocol: string, network: string, security: string, encryption?: string): boolean {
  if (protocol !== 'vless') return false;
  if (network === 'tcp' && (security === 'tls' || security === 'reality')) return true;
  return network === 'xhttp' && !!encryption && encryption !== 'none';
}

export function canEnableMux(protocol: string, flow: string, network: string): boolean {
  if (!muxProtocols.has(protocol)) return false;
  if (protocol === 'vless' && flow) return false;
  return network !== 'xhttp';
}

export function shouldShowInboundProtocolTab(
  runtimeMode: string,
  protocol: string,
  network: string,
  security: string,
): boolean {
  if (runtimeMode === 'tapx') return false;
  if (['vless', 'shadowsocks', 'http', 'mixed', 'wireguard', 'tunnel', 'tun', 'mtproto'].includes(protocol)) return true;
  return protocol === 'trojan' && network === 'tcp' && (security === 'tls' || security === 'reality');
}

export function hasSelectableXrayTransport(protocol: string): boolean {
  return !['hysteria', 'wireguard', 'tunnel'].includes(protocol);
}

export function canEnableFinalMask(protocol: string): boolean {
  return protocol !== 'tunnel';
}

export function newStreamSlice(network: string): Record<string, unknown> {
  switch (network) {
    case 'tcp':
      return { network: 'tcp', security: 'none', tcpSettings: { header: { type: 'none' } } };
    case 'kcp':
      return {
        network: 'kcp',
        security: 'none',
        kcpSettings: {
          mtu: 1350,
          tti: 20,
          uplinkCapacity: 5,
          downlinkCapacity: 20,
          cwndMultiplier: 1,
          maxSendingWindow: 2097152,
        },
      };
    case 'ws':
      return { network: 'ws', security: 'none', wsSettings: { path: '/', host: '', headers: {}, heartbeatPeriod: 0 } };
    case 'grpc':
      return { network: 'grpc', security: 'none', grpcSettings: { serviceName: '', authority: '', multiMode: false } };
    case 'httpupgrade':
      return { network: 'httpupgrade', security: 'none', httpupgradeSettings: { path: '/', host: '', headers: {} } };
    case 'xhttp':
      return { network: 'xhttp', security: 'none', xhttpSettings: { path: '/', host: '', mode: 'auto', headers: [], xPaddingBytes: '100-1000' } };
    case 'hysteria':
      return {
        network: 'hysteria',
        security: 'tls',
        hysteriaSettings: { version: 2, auth: '', udpIdleTimeout: 60 },
        tlsSettings: {
          serverName: '',
          alpn: ['h3'],
          fingerprint: '',
          echConfigList: '',
          verifyPeerCertByName: '',
          pinnedPeerCertSha256: '',
        },
      };
    default:
      return { network: 'tcp', security: 'none', tcpSettings: { header: { type: 'none' } } };
  }
}

export function newInboundStreamSlice(network: string): Record<string, unknown> {
  switch (network) {
    case 'tcp':
      return { network: 'tcp', security: 'none', tcpSettings: { acceptProxyProtocol: false, header: { type: 'none' } } };
    case 'kcp':
      return newStreamSlice('kcp');
    case 'ws':
      return {
        network: 'ws',
        security: 'none',
        wsSettings: { acceptProxyProtocol: false, path: '/', host: '', headers: {}, heartbeatPeriod: 0 },
      };
    case 'grpc':
      return newStreamSlice('grpc');
    case 'httpupgrade':
      return {
        network: 'httpupgrade',
        security: 'none',
        httpupgradeSettings: { acceptProxyProtocol: false, path: '/', host: '', headers: {} },
      };
    case 'xhttp':
      return {
        network: 'xhttp',
        security: 'none',
        xhttpSettings: {
          path: '/',
          host: '',
          mode: 'auto',
          xPaddingBytes: '100-1000',
          xPaddingObfsMode: false,
          xPaddingKey: '',
          xPaddingHeader: '',
          xPaddingPlacement: '',
          xPaddingMethod: '',
          sessionIDPlacement: '',
          sessionIDKey: '',
          sessionIDTable: '',
          sessionIDLength: '',
          seqPlacement: '',
          seqKey: '',
          uplinkDataPlacement: '',
          uplinkDataKey: '',
          scMaxEachPostBytes: '',
          noSSEHeader: false,
          scMaxBufferedPosts: 30,
          scStreamUpServerSecs: '20-80',
          serverMaxHeaderBytes: 0,
          uplinkHTTPMethod: '',
          headers: {},
          scMinPostsIntervalMs: '',
          uplinkChunkSize: 0,
          noGRPCHeader: false,
          enableXmux: false,
        },
      };
    default:
      return newStreamSlice(network);
  }
}

export function newInboundHysteriaStreamSlice(): Record<string, unknown> {
  return {
    network: 'hysteria',
    security: 'tls',
    hysteriaSettings: { version: 2, udpIdleTimeout: 60 },
    tlsSettings: newInboundTlsSettings(true),
    finalmask: {
      tcp: [],
      udp: [{
        type: 'salamander',
        settings: { password: randomText(16, 'abcdefghijklmnopqrstuvwxyz0123456789') },
      }],
    },
  };
}

export function applyNetworkChange(
  protocol: string,
  previous: Record<string, unknown> | undefined,
  nextNetwork: string,
  direction: XrayDirection = 'outbound',
): Record<string, unknown> {
  const previousSecurity = String(previous?.security || 'none');
  const security =
    previousSecurity === 'tls' && !canEnableTls(protocol, nextNetwork)
      ? 'none'
      : previousSecurity === 'reality' && !canEnableReality(protocol, nextNetwork)
        ? 'none'
        : previousSecurity;
  const next = direction === 'inbound'
    ? applyInboundNetworkDefaults(previous, nextNetwork)
    : newStreamSlice(nextNetwork);
  next.security = security;
  if (security === 'tls' && previous?.tlsSettings) next.tlsSettings = previous.tlsSettings;
  if (security === 'reality' && previous?.realitySettings) next.realitySettings = previous.realitySettings;
  return next;
}

function applyInboundNetworkDefaults(previous: Record<string, unknown> | undefined, nextNetwork: string): Record<string, unknown> {
  const next: Record<string, unknown> = { ...(previous || {}), ...newInboundStreamSlice(nextNetwork) };
  for (const key of ['tcpSettings', 'kcpSettings', 'wsSettings', 'grpcSettings', 'httpupgradeSettings', 'xhttpSettings']) {
    if (key !== `${nextNetwork}Settings`) delete next[key];
  }

  const finalmask = next.finalmask && typeof next.finalmask === 'object' && !Array.isArray(next.finalmask)
    ? { ...(next.finalmask as Record<string, unknown>) }
    : {};
  const udp = Array.isArray(finalmask.udp) ? [...finalmask.udp] : [];
  if (nextNetwork === 'kcp') {
    if (!udp.some((item) => item && typeof item === 'object' && (item as { type?: string }).type === 'mkcp-legacy')) {
      udp.push({ type: 'mkcp-legacy', settings: { header: '', value: '' } });
    }
  } else {
    for (let index = udp.length - 1; index >= 0; index -= 1) {
      const item = udp[index];
      if (item && typeof item === 'object' && (item as { type?: string }).type === 'mkcp-legacy') udp.splice(index, 1);
    }
  }
  if (Object.keys(finalmask).length > 0 || udp.length > 0) next.finalmask = { ...finalmask, udp };
  return next;
}

export function isServerProtocol(protocol: string): boolean {
  return serverProtocols.has(protocol);
}

export function XrayInboundProtocolFields({
  form,
  protocol,
  network,
  security,
  outboundTags = [],
}: {
  form: FormInstance;
  protocol: string;
  network: string;
  security: string;
  outboundTags?: string[];
}) {
  const ssMethod = Form.useWatch(['settings', 'method'], form) as string | undefined;
  const mixedUdp = Form.useWatch(['settings', 'udp'], form) === true;
  const routeThroughXray = Form.useWatch(['settings', 'routeThroughXray'], form) === true;

  if (protocol === 'vless') return <VlessInboundFields form={form} network={network} security={security} />;
  if (protocol === 'shadowsocks') return <ShadowsocksInboundFields form={form} method={ssMethod} />;
  if (protocol === 'http') return <HttpInboundFields />;
  if (protocol === 'mixed') return <MixedInboundFields mixedUdpOn={mixedUdp} />;
  if (protocol === 'wireguard') return <WireguardInboundFields form={form} />;
  if (protocol === 'tunnel') return <TunnelInboundFields />;
  if (protocol === 'tun') return <TunInboundFields />;
  if (protocol === 'mtproto') return <MtprotoInboundFields form={form} routeThroughXray={routeThroughXray} outboundTags={outboundTags} />;

  return null;
}

export function XrayOutboundProtocolFields({ form, protocol }: { form: FormInstance; protocol: string }) {
  const { t } = useI18n();
  const wireguardPeers = Form.useWatch(['settings', 'peers'], form) as unknown[] | undefined;

  return (
    <>
      {isServerProtocol(protocol) ? <ServerTargetFields /> : null}
      {protocol === 'vmess' ? <VmessOutboundFields /> : null}
      {protocol === 'vless' ? <VlessOutboundFields /> : null}
      {protocol === 'trojan' ? <TrojanOutboundFields /> : null}
      {protocol === 'shadowsocks' ? <ShadowsocksOutboundFields /> : null}
      {protocol === 'http' ? <HttpOutboundFields /> : null}
      {protocol === 'socks' ? <SocksOutboundFields /> : null}
      {protocol === 'wireguard' ? <WireguardOutboundFields form={form} peers={wireguardPeers || []} /> : null}
      {protocol === 'freedom' ? <FreedomOutboundFields form={form} /> : null}
      {protocol === 'blackhole' ? <BlackholeOutboundFields /> : null}
      {protocol === 'dns' ? <DnsOutboundFields /> : null}
      {protocol === 'loopback' ? <LoopbackOutboundFields sniffing={<SniffingFields form={form} name={['settings', 'sniffing']} label={t('xray.sniffing')} />} /> : null}
    </>
  );
}

export function XrayTransportFields({
  form,
  direction,
  protocol,
  network,
  outboundTags = [],
  includeExtras = true,
}: {
  form: FormInstance;
  direction: XrayDirection;
  protocol: string;
  network: string;
  outboundTags?: string[];
  includeExtras?: boolean;
}) {
  const { t } = useI18n();
  if (protocol === 'hysteria') {
    return (
      <>
        {direction === 'outbound' ? (
          <Form.Item label={t('xray.transport')} name={['streamSettings', 'network']}>
            <Select options={[{ value: 'hysteria', label: 'Hysteria' }]} />
          </Form.Item>
        ) : null}
        <HysteriaFields form={form} direction={direction} />
        {includeExtras ? <SockoptFields form={form} direction={direction} network={network} outboundTags={outboundTags} /> : null}
        {includeExtras && canEnableFinalMask(protocol) ? <FinalMaskFields name={['streamSettings', 'finalmask']} network={network} protocol={protocol} /> : null}
      </>
    );
  }
  if (!hasSelectableXrayTransport(protocol)) {
    if (!includeExtras) return null;
    return (
      <>
        <SockoptFields form={form} direction={direction} network={network} outboundTags={outboundTags} />
        {canEnableFinalMask(protocol) ? <FinalMaskFields name={['streamSettings', 'finalmask']} network={network} protocol={protocol} /> : null}
      </>
    );
  }

  return (
    <>
      <Form.Item label={t('xray.transport')} name={['streamSettings', 'network']}>
        <Select
          options={networkOptions}
          onChange={(nextNetwork: string) => form.setFieldValue('streamSettings', applyNetworkChange(protocol, form.getFieldValue('streamSettings'), nextNetwork, direction))}
        />
      </Form.Item>
      {network === 'tcp' ? <RawTransportFields form={form} direction={direction} /> : null}
      {network === 'kcp' ? <KcpTransportFields direction={direction} /> : null}
      {network === 'ws' ? <WsTransportFields direction={direction} /> : null}
      {network === 'grpc' ? <GrpcTransportFields /> : null}
      {network === 'httpupgrade' ? <HttpUpgradeTransportFields direction={direction} /> : null}
      {network === 'xhttp' ? <XhttpTransportFields form={form} direction={direction} /> : null}
      {includeExtras ? <SockoptFields form={form} direction={direction} network={network} outboundTags={outboundTags} /> : null}
      {includeExtras && canEnableFinalMask(protocol) ? <FinalMaskFields name={['streamSettings', 'finalmask']} network={network} protocol={protocol} /> : null}
    </>
  );
}

export function XraySecurityFields({
  form,
  direction,
  protocol,
  network,
  security,
  panelCertificate,
}: {
  form: FormInstance;
  direction: XrayDirection;
  protocol: string;
  network: string;
  security: string;
  panelCertificate?: { certPublicPath?: string; certPrivatePath?: string };
}) {
  const { t } = useI18n();
  const tlsAllowed = canEnableTls(protocol, network);
  const realityAllowed = canEnableReality(protocol, network);
  const tlsOnly = protocol === 'hysteria';

  async function setSecurity(next: string) {
    const stream = { ...(form.getFieldValue('streamSettings') || {}) };
    delete stream.tlsSettings;
    delete stream.realitySettings;
    stream.security = next;
    if (next === 'tls') stream.tlsSettings = direction === 'inbound' ? newInboundTlsSettings() : newOutboundTlsSettings();
    if (next === 'reality') {
      stream.realitySettings = direction === 'inbound'
        ? newInboundRealitySettings()
        : newOutboundRealitySettings();
    }
    form.setFieldValue('streamSettings', stream);
    if (next === 'reality' && direction === 'inbound') {
      try {
        const pair = await getPanelObject<{ privateKey?: string; publicKey?: string }>('/api/xray/reality/x25519');
        form.setFieldValue(['streamSettings', 'realitySettings', 'privateKey'], pair.privateKey || '');
        form.setFieldValue(['streamSettings', 'realitySettings', 'settings', 'publicKey'], pair.publicKey || '');
      } catch {
        // The generation button remains available when the initial best-effort request fails.
      }
    }
  }

  return (
    <>
      <Form.Item name={['streamSettings', 'security']} label={t('xray.security')}>
        <Radio.Group buttonStyle="solid" disabled={!tlsAllowed} onChange={(event) => setSecurity(event.target.value)}>
          {!tlsOnly ? <Radio.Button value="none">{t('xray.none')}</Radio.Button> : null}
          {tlsAllowed ? <Radio.Button value="tls">TLS</Radio.Button> : null}
          {realityAllowed ? <Radio.Button value="reality">Reality</Radio.Button> : null}
        </Radio.Group>
      </Form.Item>
      {security === 'tls' && tlsAllowed
        ? direction === 'inbound'
          ? <XrayInboundTlsFields form={form} panelCertificate={panelCertificate} />
          : <XrayOutboundTlsFields />
        : null}
      {security === 'reality' && realityAllowed
        ? direction === 'inbound'
          ? <XrayInboundRealityFields form={form} />
          : <XrayOutboundRealityFields />
        : null}
    </>
  );
}

export function XrayMuxFields({ form, protocol, network }: { form: FormInstance; protocol: string; network: string }) {
  const flow = Form.useWatch(['settings', 'flow'], form) as string | undefined;
  if (!canEnableMux(protocol, flow || '', network)) return null;
  return <MuxFields form={form} />;
}
