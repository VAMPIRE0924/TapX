import { Alert, Form, Input, InputNumber, Segmented, Select, Switch, type FormInstance } from 'antd';
import { targetStrategyOptions } from '../options';
import type { XrayDirection } from '../XrayFormFields';
import { CustomSockoptList } from './CustomSockoptList';
import { useI18n } from '../../../i18n/I18nProvider';

const transportProxyField: Record<string, string> = {
  tcp: 'tcpSettings',
  ws: 'wsSettings',
  httpupgrade: 'httpupgradeSettings',
};

const trustedHeaderNetworks = new Set(['ws', 'httpupgrade', 'xhttp', 'grpc']);
type RealClientIpPreset = 'off' | 'cloudflare' | 'proxy';

export function SockoptFields({
  form,
  direction,
  network,
  outboundTags = [],
}: {
  form: FormInstance;
  direction: XrayDirection;
  network: string;
  outboundTags?: string[];
}) {
  return (
    <Form.Item shouldUpdate noStyle>
      {() => {
        const hasSockopt = !!form.getFieldValue(['streamSettings', 'sockopt']);
        const toggleSockopt = (checked: boolean) => {
          form.setFieldValue(['streamSettings', 'sockopt'], checked ? defaultSockopt() : undefined);
        };
        return (
          <>
            <Form.Item label={direction === 'outbound' ? 'Sockopts' : 'Sockopt'}>
              <Switch checked={hasSockopt} onChange={toggleSockopt} aria-label={direction === 'outbound' ? 'Sockopts' : 'Sockopt'} />
            </Form.Item>
            {hasSockopt ? (
              direction === 'outbound'
                ? <OutboundSockoptFields form={form} outboundTags={outboundTags} />
                : <InboundSockoptFields form={form} network={network} toggleSockopt={toggleSockopt} />
            ) : null}
          </>
        );
      }}
    </Form.Item>
  );
}

function defaultSockopt(): Record<string, unknown> {
  return {
    acceptProxyProtocol: false,
    tcpFastOpen: false,
    mark: 0,
    tproxy: 'off',
    tcpMptcp: false,
    penetrate: false,
    domainStrategy: 'AsIs',
    tcpMaxSeg: 0,
    dialerProxy: '',
    tcpKeepAliveInterval: 0,
    tcpKeepAliveIdle: 0,
    tcpUserTimeout: 0,
    tcpcongestion: 'bbr',
    V6Only: false,
    tcpWindowClamp: 0,
    interface: '',
    trustedXForwardedFor: [],
    addressPortStrategy: 'none',
    customSockopt: [],
  };
}

function InboundSockoptFields({
  form,
  network,
  toggleSockopt,
}: {
  form: FormInstance;
  network: string;
  toggleSockopt: (checked: boolean) => void;
}) {
  const { t } = useI18n();
  return (
    <>
      <InboundRealClientIpFields form={form} network={network} toggleSockopt={toggleSockopt} />
      <Form.Item label="Route Mark" name={['streamSettings', 'sockopt', 'mark']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label={t('xray.tcpKeepAliveInterval')} name={['streamSettings', 'sockopt', 'tcpKeepAliveInterval']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP Keep Alive Idle" name={['streamSettings', 'sockopt', 'tcpKeepAliveIdle']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP Max Seg" name={['streamSettings', 'sockopt', 'tcpMaxSeg']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP User Timeout" name={['streamSettings', 'sockopt', 'tcpUserTimeout']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP Window Clamp" name={['streamSettings', 'sockopt', 'tcpWindowClamp']} tooltip={t('xray.tcpWindowClampHelpDetailed')}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="Proxy Protocol" name={['streamSettings', 'sockopt', 'acceptProxyProtocol']} tooltip={t('xray.proxyProtocolHelp')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="TCP Fast Open" name={['streamSettings', 'sockopt', 'tcpFastOpen']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="Penetrate" name={['streamSettings', 'sockopt', 'penetrate']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label={t('xray.v6Only')} name={['streamSettings', 'sockopt', 'V6Only']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <TcpCongestionField />
      <TproxyField />
      <TrustedXForwardedForField />
      <CustomSockoptList />
    </>
  );
}

function InboundRealClientIpFields({
  form,
  network,
  toggleSockopt,
}: {
  form: FormInstance;
  network: string;
  toggleSockopt: (checked: boolean) => void;
}) {
  const { t } = useI18n();
  const sockopt = (Form.useWatch(['streamSettings', 'sockopt'], form) || {}) as Record<string, unknown>;
  const transportField = transportProxyField[network];
  const transportProxy = transportField
    ? form.getFieldValue(['streamSettings', transportField, 'acceptProxyProtocol']) === true
    : false;
  const proxyOn = sockopt.acceptProxyProtocol === true || transportProxy;
  const trusted = Array.isArray(sockopt.trustedXForwardedFor) ? sockopt.trustedXForwardedFor as string[] : [];
  const value: RealClientIpPreset = proxyOn ? 'proxy' : trusted.length > 0 ? 'cloudflare' : 'off';
  const trustedMismatch = trusted.length > 0 && !trustedHeaderNetworks.has(network);
  const proxyMismatch = proxyOn && network === 'kcp';

  function applyPreset(preset: RealClientIpPreset) {
    if (preset !== 'off' && !form.getFieldValue(['streamSettings', 'sockopt'])) toggleSockopt(true);
    if (preset === 'off') {
      form.setFieldValue(['streamSettings', 'sockopt', 'trustedXForwardedFor'], []);
      form.setFieldValue(['streamSettings', 'sockopt', 'acceptProxyProtocol'], false);
      if (transportField) form.setFieldValue(['streamSettings', transportField, 'acceptProxyProtocol'], false);
      return;
    }
    if (preset === 'cloudflare') {
      const next = [...trusted];
      if (!next.includes('CF-Connecting-IP')) next.push('CF-Connecting-IP');
      form.setFieldValue(['streamSettings', 'sockopt', 'trustedXForwardedFor'], next);
      form.setFieldValue(['streamSettings', 'sockopt', 'acceptProxyProtocol'], false);
      if (transportField) form.setFieldValue(['streamSettings', transportField, 'acceptProxyProtocol'], false);
      return;
    }
    form.setFieldValue(['streamSettings', 'sockopt', 'trustedXForwardedFor'], []);
    form.setFieldValue(['streamSettings', 'sockopt', 'acceptProxyProtocol'], true);
    if (transportField) form.setFieldValue(['streamSettings', transportField, 'acceptProxyProtocol'], true);
  }

  return (
    <>
      <Form.Item label={t('xray.realClientIp')} tooltip={t('xray.realClientIpHelp')}>
        <Segmented
          value={value}
          onChange={(next) => applyPreset(next as RealClientIpPreset)}
          options={[
            { value: 'off', label: t('xray.offDirect') },
            { value: 'cloudflare', label: 'Cloudflare CDN' },
            { value: 'proxy', label: t('xray.l4Relay') },
          ]}
        />
      </Form.Item>
      {trustedMismatch ? <Alert type="warning" showIcon style={{ marginBottom: 16 }} title={t('xray.trustedXffMismatch')} /> : null}
      {proxyMismatch ? <Alert type="warning" showIcon style={{ marginBottom: 16 }} title={t('xray.proxyProtocolMismatch')} /> : null}
    </>
  );
}

function OutboundSockoptFields({ form, outboundTags }: { form: FormInstance; outboundTags: string[] }) {
  const { t } = useI18n();
  const dialerProxy = String(Form.useWatch(['streamSettings', 'sockopt', 'dialerProxy'], form) || '');
  const dialerProxyOptions = Array.from(new Set([...outboundTags, dialerProxy].filter(Boolean))).map((value) => ({ value, label: value }));

  return (
    <>
      <Form.Item
        label="Dialer Proxy"
        name={['streamSettings', 'sockopt', 'dialerProxy']}
        tooltip={t('xray.dialerProxyHelp')}
      >
        <Select allowClear showSearch placeholder={t('xray.selectChainedConnector')} options={dialerProxyOptions} />
      </Form.Item>
      <Form.Item label="Domain Strategy" name={['streamSettings', 'sockopt', 'domainStrategy']}>
        <Select options={targetStrategyOptions} />
      </Form.Item>
      <Form.Item label="Address Port Strategy" name={['streamSettings', 'sockopt', 'addressPortStrategy']}>
        <Select options={addressPortStrategyOptions} />
      </Form.Item>
      <Form.Item label={t('xray.keepAliveInterval')} name={['streamSettings', 'sockopt', 'tcpKeepAliveInterval']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP Fast Open" name={['streamSettings', 'sockopt', 'tcpFastOpen']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="Multipath TCP" name={['streamSettings', 'sockopt', 'tcpMptcp']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="Penetrate" name={['streamSettings', 'sockopt', 'penetrate']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="Mark (fwmark)" name={['streamSettings', 'sockopt', 'mark']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="Interface" name={['streamSettings', 'sockopt', 'interface']}>
        <Input />
      </Form.Item>
      <TproxyField outbound />
      <TcpCongestionField outbound />
      <Form.Item label="TCP user timeout (ms)" name={['streamSettings', 'sockopt', 'tcpUserTimeout']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP keep-alive idle (s)" name={['streamSettings', 'sockopt', 'tcpKeepAliveIdle']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label="TCP Max Seg" name={['streamSettings', 'sockopt', 'tcpMaxSeg']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item
        label="TCP Window Clamp"
        name={['streamSettings', 'sockopt', 'tcpWindowClamp']}
        tooltip={t('xray.tcpWindowClampHelp')}
      >
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item shouldUpdate noStyle>
        {() => {
          const happyEyeballs = !!form.getFieldValue(['streamSettings', 'sockopt', 'happyEyeballs']);
          return (
            <>
              <Form.Item label="Happy Eyeballs">
                <Switch
                  aria-label="Happy Eyeballs"
                  checked={happyEyeballs}
                  onChange={(checked) => form.setFieldValue(['streamSettings', 'sockopt', 'happyEyeballs'], checked ? defaultHappyEyeballs() : undefined)}
                />
              </Form.Item>
              {happyEyeballs ? <HappyEyeballsFields /> : null}
            </>
          );
        }}
      </Form.Item>
      <CustomSockoptList />
    </>
  );
}

function defaultHappyEyeballs(): Record<string, unknown> {
  return {
    tryDelayMs: 0,
    prioritizeIPv6: false,
    interleave: 1,
    maxConcurrentTry: 4,
  };
}

const addressPortStrategyOptions = [
  'none',
  'SrvPortOnly',
  'SrvAddressOnly',
  'SrvPortAndAddress',
  'TxtPortOnly',
  'TxtAddressOnly',
  'TxtPortAndAddress',
].map((value) => ({ value, label: value }));

function HappyEyeballsFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.tryDelayMs')} name={['streamSettings', 'sockopt', 'happyEyeballs', 'tryDelayMs']}>
        <InputNumber min={0} placeholder="0 (disabled) — 250 recommended" />
      </Form.Item>
      <Form.Item label={t('xray.prioritizeIpv6')} name={['streamSettings', 'sockopt', 'happyEyeballs', 'prioritizeIPv6']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="Interleave" name={['streamSettings', 'sockopt', 'happyEyeballs', 'interleave']}>
        <InputNumber min={1} />
      </Form.Item>
      <Form.Item label={t('xray.maxConcurrentTry')} name={['streamSettings', 'sockopt', 'happyEyeballs', 'maxConcurrentTry']}>
        <InputNumber min={0} />
      </Form.Item>
    </>
  );
}

function TcpCongestionField({ outbound = false }: { outbound?: boolean } = {}) {
  return (
    <Form.Item label="TCP Congestion" name={['streamSettings', 'sockopt', 'tcpcongestion']}>
      <Select style={outbound ? undefined : { width: '50%' }} allowClear options={['bbr', 'cubic', 'reno'].map((value) => ({ value, label: value }))} />
    </Form.Item>
  );
}

function TproxyField({ outbound = false }: { outbound?: boolean } = {}) {
  return (
    <Form.Item label="TProxy" name={['streamSettings', 'sockopt', 'tproxy']}>
      <Select
        style={outbound ? undefined : { width: '50%' }}
        options={[
          { value: 'off', label: outbound ? 'off' : 'Off' },
          { value: 'redirect', label: outbound ? 'redirect' : 'Redirect' },
          { value: 'tproxy', label: outbound ? 'tproxy' : 'TProxy' },
        ]}
      />
    </Form.Item>
  );
}

function TrustedXForwardedForField() {
  const { t } = useI18n();
  return (
    <Form.Item
      label={t('xray.trustedXff')}
      name={['streamSettings', 'sockopt', 'trustedXForwardedFor']}
      tooltip={t('xray.trustedXffHelp')}
    >
      <Select
        mode="tags"
        tokenSeparators={[',']}
        options={['CF-Connecting-IP', 'X-Real-IP', 'True-Client-IP', 'X-Client-IP'].map((value) => ({ value, label: value }))}
      />
    </Form.Item>
  );
}
