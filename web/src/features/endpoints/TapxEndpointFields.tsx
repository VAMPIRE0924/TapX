import { Button, Form, Input, InputNumber, Radio, Select, Space, Switch } from 'antd';
import { UnitInputNumber } from '../../components/UnitInputNumber';
import { useI18n } from '../../i18n/I18nProvider';

export interface PanelCertificate {
  certPublicPath?: string;
  certPrivatePath?: string;
}

export function TapxListenerFastPathFields({ tcp }: { tcp: boolean }) {
  return <TapxRawTransportFields tcp={tcp} />;
}

export function TapxConnectorFastPathFields({ tcp }: { tcp: boolean }) {
  return <TapxRawTransportFields tcp={tcp} />;
}

function TapxRawTransportFields({ tcp }: { tcp: boolean }) {
  const { t } = useI18n();
  const base = tcp ? 'RawTCP' : 'RawUDP';

  return (
    <>
      {!tcp ? (
        <Form.Item name={['RawUDP', 'KeepAliveSecond']} label={t('endpoint.keepAliveInterval')} tooltip={t('endpoint.keepAliveHelp')}>
          <UnitInputNumber min={0} unit="s" placeholder="15" style={{ width: '100%' }} />
        </Form.Item>
      ) : null}
      <Form.Item name={[base, 'Workers']} label={t('endpoint.workerThreads')} tooltip={t('endpoint.workerThreadsHelp')}>
        <InputNumber min={0} placeholder="0" />
      </Form.Item>
      <Form.Item name={[base, 'QueueSize']} label={t('endpoint.queueSize')} tooltip={t('endpoint.queueSizeHelp')}>
        <InputNumber min={0} placeholder="0" />
      </Form.Item>
      <Form.Item name={[base, 'ZeroCopy']} label="Zero-copy" valuePropName="checked" tooltip={t('endpoint.zeroCopyHelp')}>
        <Switch />
      </Form.Item>
      <Form.Item name={[base, 'ConnectTimeout']} label={t('endpoint.connectTimeout')} tooltip={t('endpoint.connectTimeoutHelp')}>
        <UnitInputNumber min={0} unit="s" placeholder="10" style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item name={[base, 'IdleTimeout']} label={t('endpoint.idleTimeout')} tooltip={t('endpoint.idleTimeoutHelp')}>
        <UnitInputNumber min={0} unit="s" placeholder="60" style={{ width: '100%' }} />
      </Form.Item>
      {tcp ? (
        <>
          <Form.Item name={['RawTCP', 'LengthMode']} label={t('endpoint.tcpLengthMode')} tooltip={t('endpoint.tcpLengthModeHelp')}>
            <Select options={tcpLengthModeOptions} />
          </Form.Item>
          <Form.Item name={['RawTCP', 'NoDelay']} label="TCP NoDelay" tooltip={t('endpoint.noDelayHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name={['RawTCP', 'KeepAliveSecond']} label={t('endpoint.keepAliveInterval')} tooltip={t('endpoint.keepAliveHelp')}>
            <UnitInputNumber min={0} unit="s" placeholder="15" style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name={['RawTCP', 'FastOpen']} label="TCP Fast Open" tooltip={t('endpoint.fastOpenHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name={['RawTCP', 'ReconnectSecond']} label={t('endpoint.reconnectInterval')} tooltip={t('endpoint.reconnectHelp')}>
            <UnitInputNumber min={0} unit="s" placeholder="3" style={{ width: '100%' }} />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}

const tcpLengthModeOptions = [
  { value: 'uint16', label: 'uint16' },
  { value: 'uint32', label: 'uint32' },
];

export function TapxListenerTlsFields({ panelCertificate }: { panelCertificate?: PanelCertificate }) {
  return <TapxServerSecurityFields mode="tls" panelCertificate={panelCertificate} />;
}

export function TapxListenerDtlsFields({ panelCertificate }: { panelCertificate?: PanelCertificate }) {
  return <TapxServerSecurityFields mode="dtls" panelCertificate={panelCertificate} />;
}

function TapxServerSecurityFields({ mode, panelCertificate }: { mode: 'tls' | 'dtls'; panelCertificate?: PanelCertificate }) {
  const form = Form.useFormInstance();
  const { t } = useI18n();
  const tlsVersionOptions = useTlsVersionOptions();

  function usePanelCertificate() {
    form.setFieldValue(['TLS', 'CertFile'], panelCertificate?.certPublicPath || '');
    form.setFieldValue(['TLS', 'KeyFile'], panelCertificate?.certPrivatePath || '');
  }

  function clearCertificate() {
    form.setFieldValue(['TLS', 'CertFile'], '');
    form.setFieldValue(['TLS', 'KeyFile'], '');
  }

  return (
    <>
      <Form.Item name={['TLS', 'ServerName']} label="SNI" tooltip={t('endpoint.serverNameHelp')}>
        <Input placeholder="tapx.example.com" />
      </Form.Item>
      <Form.Item label={t('endpoint.minMaxVersion')} tooltip={t('endpoint.tlsVersionHelp')}>
        <Space.Compact block>
          <Form.Item name={['TLS', 'MinVersion']} noStyle>
            <Select options={tlsVersionOptions} />
          </Form.Item>
          <Form.Item name={['TLS', 'MaxVersion']} noStyle>
            <Select options={tlsVersionOptions} />
          </Form.Item>
        </Space.Compact>
      </Form.Item>
      <Form.Item label={t('endpoint.certificateOne')}>
        <Radio.Group buttonStyle="solid" value="file">
          <Radio.Button value="file">{t('endpoint.filePath')}</Radio.Button>
        </Radio.Group>
      </Form.Item>
      <Form.Item name={['TLS', 'CertFile']} label={t('endpoint.publicKey')} tooltip={t('endpoint.certFileHelp')}>
        <Input placeholder="/etc/ssl/tapx/fullchain.pem" />
      </Form.Item>
      <Form.Item name={['TLS', 'KeyFile']} label={t('endpoint.privateKey')} tooltip={t('endpoint.keyFileHelp')}>
        <Input placeholder="/etc/ssl/tapx/privkey.pem" />
      </Form.Item>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" onClick={usePanelCertificate}>
            {t('endpoint.usePanelCertificate')}
          </Button>
          <Button danger onClick={clearCertificate}>
            {t('endpoint.clear')}
          </Button>
        </Space>
      </Form.Item>
      {mode === 'dtls' ? (
        <>
          <Form.Item name={['TLS', 'DtlsMtu']} label="DTLS MTU" tooltip={t('endpoint.dtlsMtuHelp')}>
            <InputNumber min={0} placeholder="1200" />
          </Form.Item>
          <Form.Item name={['TLS', 'DtlsReplayWindow']} label={t('endpoint.dtlsReplayWindow')} tooltip={t('endpoint.dtlsReplayHelp')}>
            <InputNumber min={0} placeholder="64" />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}

export function TapxConnectorTlsFields() {
  return <TapxClientSecurityFields mode="tls" />;
}

export function TapxConnectorDtlsFields() {
  return <TapxClientSecurityFields mode="dtls" />;
}

function TapxClientSecurityFields({ mode }: { mode: 'tls' | 'dtls' }) {
  const { t } = useI18n();
  const tlsVersionOptions = useTlsVersionOptions();
  return (
    <>
      <Form.Item name={['TLS', 'ServerName']} label="SNI / ServerName" tooltip={t('endpoint.serverNameHelp')}>
        <Input placeholder="tapx.example.com" />
      </Form.Item>
      <Form.Item name={['TLS', 'MinVersion']} label={t('endpoint.minTlsVersion')} tooltip={t('endpoint.tlsVersionHelp')}>
        <Select options={tlsVersionOptions} />
      </Form.Item>
      <Form.Item name={['TLS', 'MaxVersion']} label={t('endpoint.maxTlsVersion')} tooltip={t('endpoint.tlsVersionHelp')}>
        <Select options={tlsVersionOptions} />
      </Form.Item>
      <Form.Item
        name={['TLS', 'AllowInsecure']}
        label={t('endpoint.allowInsecure')}
        tooltip={t('endpoint.allowInsecureHelp')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      {mode === 'dtls' ? (
        <>
          <Form.Item name={['TLS', 'DtlsMtu']} label="DTLS MTU" tooltip={t('endpoint.dtlsMtuHelp')}>
            <InputNumber min={0} placeholder="1200" />
          </Form.Item>
          <Form.Item name={['TLS', 'DtlsReplayWindow']} label={t('endpoint.dtlsReplayWindow')} tooltip={t('endpoint.dtlsReplayHelp')}>
            <InputNumber min={0} placeholder="64" />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}

function useTlsVersionOptions() {
  const { t } = useI18n();
  return ['', '1.0', '1.1', '1.2', '1.3'].map((value) => ({
    value,
    label: value || t('endpoint.default'),
  }));
}
