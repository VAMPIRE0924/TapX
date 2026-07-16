import { useState } from 'react';
import { CloudDownloadOutlined, FileProtectOutlined, MinusOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input, InputNumber, message, Radio, Select, Space, Switch, type FormInstance } from 'antd';
import { alpnOptions, getTlsCipherOptions, getTlsVersionOptions, getUtlsOptions, targetStrategyOptions, tlsUsageOptions } from '../options';
import { useI18n } from '../../../i18n/I18nProvider';
import { postPanelObject } from './api';
import { newInboundCertificate } from './defaults';

export interface PanelCertificate {
  certPublicPath?: string;
  certPrivatePath?: string;
}

const curvePreferenceOptions = ['X25519MLKEM768', 'X25519', 'P-256', 'P-384', 'P-521'].map((value) => ({
  value,
  label: value,
}));

function defaultEchSockopt() {
  return {
    dialerProxy: '',
    domainStrategy: 'AsIs',
    tcpFastOpen: false,
    tcpMptcp: false,
  };
}

function splitLines(value: unknown) {
  return typeof value === 'string' ? value.split('\n') : value;
}

function textAreaValue(value: unknown) {
  return { value: Array.isArray(value) ? value.join('\n') : value };
}

export function XrayInboundTlsFields({
  form,
  panelCertificate,
}: {
  form: FormInstance;
  panelCertificate?: PanelCertificate;
}) {
  const { t } = useI18n();
  const xrayTlsVersionOptions = getTlsVersionOptions(t).filter((option) => option.value);
  const xrayUtlsOptions = getUtlsOptions(t);
  const tlsCipherOptions = getTlsCipherOptions(t);
  const [echLoading, setEchLoading] = useState(false);
  const [pinLoading, setPinLoading] = useState<'local' | 'remote' | null>(null);

  const setCertFromPanel = (index: number) => {
    form.setFieldValue(['streamSettings', 'tlsSettings', 'certificates', index, 'useFile'], true);
    form.setFieldValue(
      ['streamSettings', 'tlsSettings', 'certificates', index, 'certificateFile'],
      panelCertificate?.certPublicPath || '',
    );
    form.setFieldValue(
      ['streamSettings', 'tlsSettings', 'certificates', index, 'keyFile'],
      panelCertificate?.certPrivatePath || '',
    );
  };

  const clearCertificate = (index: number) => {
    form.setFieldValue(['streamSettings', 'tlsSettings', 'certificates', index, 'certificateFile'], '');
    form.setFieldValue(['streamSettings', 'tlsSettings', 'certificates', index, 'keyFile'], '');
    form.setFieldValue(['streamSettings', 'tlsSettings', 'certificates', index, 'certificate'], []);
    form.setFieldValue(['streamSettings', 'tlsSettings', 'certificates', index, 'key'], []);
  };

  const generateEch = async () => {
    setEchLoading(true);
    try {
      const result = await postPanelObject<{ echServerKeys?: string; echConfigList?: string }>('/api/xray/tls/ech', {
        sni: form.getFieldValue(['streamSettings', 'tlsSettings', 'serverName']) || '',
      });
      form.setFieldValue(['streamSettings', 'tlsSettings', 'echServerKeys'], result.echServerKeys || '');
      form.setFieldValue(['streamSettings', 'tlsSettings', 'settings', 'echConfigList'], result.echConfigList || '');
      message.success(t('xray.echGenerated'));
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setEchLoading(false);
    }
  };

  const mergePinnedHashes = (hashes: string[]) => {
    const path = ['streamSettings', 'tlsSettings', 'settings', 'pinnedPeerCertSha256'];
    const current = form.getFieldValue(path);
    form.setFieldValue(path, Array.from(new Set([...(Array.isArray(current) ? current : []), ...hashes])));
  };

  const pinFromCertificate = async () => {
    const certificates = form.getFieldValue(['streamSettings', 'tlsSettings', 'certificates']);
    const first = Array.isArray(certificates) ? certificates[0] : undefined;
    const certFile = String(first?.certificateFile || '').trim();
    const certContent = Array.isArray(first?.certificate) ? first.certificate.join('\n').trim() : '';
    if (!certFile && !certContent) {
      message.warning(t('xray.certificateRequired'));
      return;
    }
    setPinLoading('local');
    try {
      mergePinnedHashes(await postPanelObject<string[]>('/api/xray/tls/cert-hash', { certFile, certContent }));
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setPinLoading(null);
    }
  };

  const pinFromRemote = async () => {
    const serverName = String(form.getFieldValue(['streamSettings', 'tlsSettings', 'serverName']) || '').trim();
    if (!serverName) {
      message.warning(t('xray.sniRequired'));
      return;
    }
    const port = Number(form.getFieldValue('port')) || 0;
    const server = /:\d+$/.test(serverName) || !port ? serverName : `${serverName}:${port}`;
    setPinLoading('remote');
    try {
      mergePinnedHashes(await postPanelObject<string[]>('/api/xray/tls/remote-cert-hash', { server }));
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setPinLoading(null);
    }
  };

  return (
    <>
      <Form.Item name={['streamSettings', 'tlsSettings', 'serverName']} label="SNI">
        <Input placeholder="SNI" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'cipherSuites']} label="Cipher Suites">
        <Select options={tlsCipherOptions} />
      </Form.Item>
      <Form.Item label={t('endpoint.minMaxVersion')}>
        <Space.Compact block>
          <Form.Item name={['streamSettings', 'tlsSettings', 'minVersion']} noStyle>
            <Select style={{ width: '50%' }} options={xrayTlsVersionOptions} />
          </Form.Item>
          <Form.Item name={['streamSettings', 'tlsSettings', 'maxVersion']} noStyle>
            <Select style={{ width: '50%' }} options={xrayTlsVersionOptions} />
          </Form.Item>
        </Space.Compact>
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'settings', 'fingerprint']} label="uTLS">
        <Select options={xrayUtlsOptions} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'alpn']} label="ALPN">
        <Select mode="multiple" tokenSeparators={[',']} options={alpnOptions} />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'tlsSettings', 'curvePreferences']}
        label={t('xray.curvePreferences')}
        tooltip={t('xray.curvePreferencesHelp')}
      >
        <Select mode="tags" tokenSeparators={[',', ' ']} options={curvePreferenceOptions} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'rejectUnknownSni']} label={t('xray.rejectUnknownSni')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'disableSystemRoot']} label={t('xray.disableSystemRoot')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'enableSessionResumption']} label={t('xray.sessionResumption')} valuePropName="checked">
        <Switch />
      </Form.Item>

      <Form.List name={['streamSettings', 'tlsSettings', 'certificates']}>
        {(certFields, { add, remove }) => (
          <>
            <Form.Item label={t('xray.certificates')}>
              <Button
                aria-label={t('xray.addCertificate')}
                type="primary"
                size="small"
                icon={<PlusOutlined />}
                onClick={() => add(newInboundCertificate())}
              />
            </Form.Item>
            {certFields.map((certField, index) => (
              <div key={certField.key}>
                <Form.Item name={[certField.name, 'useFile']} label={t('xray.certificateIndex', { index: index + 1 })}>
                  <Radio.Group buttonStyle="solid">
                    <Radio.Button value>{t('endpoint.filePath')}</Radio.Button>
                    <Radio.Button value={false}>{t('xray.fileContent')}</Radio.Button>
                  </Radio.Group>
                </Form.Item>
                {certFields.length > 1 ? (
                  <Form.Item label=" ">
                    <Button size="small" danger onClick={() => remove(certField.name)}>
                      <MinusOutlined /> {t('common.delete')}
                    </Button>
                  </Form.Item>
                ) : null}
                <Form.Item
                  noStyle
                  shouldUpdate={(prev, curr) =>
                    prev.streamSettings?.tlsSettings?.certificates?.[certField.name]?.useFile
                    !== curr.streamSettings?.tlsSettings?.certificates?.[certField.name]?.useFile
                  }
                >
                  {({ getFieldValue }) => {
                    const useFile = getFieldValue([
                      'streamSettings',
                      'tlsSettings',
                      'certificates',
                      certField.name,
                      'useFile',
                    ]);
                    return useFile !== false ? (
                      <>
                        <Form.Item name={[certField.name, 'certificateFile']} label={t('xray.publicKey')}>
                          <Input />
                        </Form.Item>
                        <Form.Item name={[certField.name, 'keyFile']} label={t('xray.privateKey')}>
                          <Input />
                        </Form.Item>
                        <Form.Item label=" ">
                          <Space>
                            <Button type="primary" onClick={() => setCertFromPanel(certField.name)}>
                              {t('endpoint.usePanelCertificate')}
                            </Button>
                            <Button danger onClick={() => clearCertificate(certField.name)}>
                              {t('xray.clear')}
                            </Button>
                          </Space>
                        </Form.Item>
                      </>
                    ) : (
                      <>
                        <Form.Item
                          name={[certField.name, 'certificate']}
                          label={t('xray.publicKey')}
                          normalize={splitLines}
                          getValueProps={textAreaValue}
                        >
                          <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} />
                        </Form.Item>
                        <Form.Item
                          name={[certField.name, 'key']}
                          label={t('xray.privateKey')}
                          normalize={splitLines}
                          getValueProps={textAreaValue}
                        >
                          <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} />
                        </Form.Item>
                      </>
                    );
                  }}
                </Form.Item>
                <Form.Item name={[certField.name, 'ocspStapling']} label="OCSP Stapling">
                  <InputNumber min={0} suffix="s" style={{ width: '50%' }} />
                </Form.Item>
                <Form.Item name={[certField.name, 'oneTimeLoading']} label={t('xray.oneTimeLoading')} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name={[certField.name, 'usage']} label={t('xray.usageOptions')}>
                  <Select style={{ width: '50%' }} options={tlsUsageOptions} />
                </Form.Item>
                <Form.Item
                  noStyle
                  shouldUpdate={(prev, curr) =>
                    prev.streamSettings?.tlsSettings?.certificates?.[certField.name]?.usage
                    !== curr.streamSettings?.tlsSettings?.certificates?.[certField.name]?.usage
                  }
                >
                  {({ getFieldValue }) =>
                    getFieldValue([
                      'streamSettings',
                      'tlsSettings',
                      'certificates',
                      certField.name,
                      'usage',
                    ]) === 'issue' ? (
                      <Form.Item name={[certField.name, 'buildChain']} label={t('xray.buildCertificateChain')} valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    ) : null}
                </Form.Item>
              </div>
            ))}
          </>
        )}
      </Form.List>

      <Form.Item
        name={['streamSettings', 'tlsSettings', 'masterKeyLog']}
        label={t('xray.masterKeyLog')}
        tooltip={t('xray.masterKeyLogHelp')}
      >
        <Input placeholder="/path/to/sslkeylog.txt" />
      </Form.Item>
      <Form.Item
        noStyle
        shouldUpdate={(prev, curr) =>
          !!prev.streamSettings?.tlsSettings?.echSockopt !== !!curr.streamSettings?.tlsSettings?.echSockopt
        }
      >
        {({ getFieldValue, setFieldValue }) => {
          const enabled = !!getFieldValue(['streamSettings', 'tlsSettings', 'echSockopt']);
          return (
            <>
              <Form.Item
                label="ECH Sockopt"
                tooltip={t('xray.echSockoptHelp')}
              >
                <Switch
                  aria-label="ECH Sockopt"
                  checked={enabled}
                  onChange={(checked) =>
                    setFieldValue(['streamSettings', 'tlsSettings', 'echSockopt'], checked ? defaultEchSockopt() : undefined)
                  }
                />
              </Form.Item>
              {enabled ? (
                <>
                  <Form.Item name={['streamSettings', 'tlsSettings', 'echSockopt', 'dialerProxy']} label="Dialer Proxy">
                    <Input />
                  </Form.Item>
                  <Form.Item name={['streamSettings', 'tlsSettings', 'echSockopt', 'domainStrategy']} label="Domain Strategy">
                    <Select options={targetStrategyOptions} />
                  </Form.Item>
                  <Form.Item name={['streamSettings', 'tlsSettings', 'echSockopt', 'tcpFastOpen']} label="TCP Fast Open" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                  <Form.Item name={['streamSettings', 'tlsSettings', 'echSockopt', 'tcpMptcp']} label="Multipath TCP" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                </>
              ) : null}
            </>
          );
        }}
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'echServerKeys']} label="ECH Key">
        <Input />
      </Form.Item>
      <Form.Item name={['streamSettings', 'tlsSettings', 'settings', 'echConfigList']} label="ECH Config">
        <Input />
      </Form.Item>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={echLoading} onClick={generateEch}>
            {t('xray.getNewEchCertificate')}
          </Button>
          <Button
            danger
            onClick={() => {
              form.setFieldValue(['streamSettings', 'tlsSettings', 'echServerKeys'], '');
              form.setFieldValue(['streamSettings', 'tlsSettings', 'settings', 'echConfigList'], '');
            }}
          >
            {t('xray.clear')}
          </Button>
        </Space>
      </Form.Item>
      <Form.Item
        label={t('xray.pinnedPeerSha256')}
        tooltip={t('xray.pinnedPeerSha256Help')}
      >
        <Space.Compact block>
          <Form.Item name={['streamSettings', 'tlsSettings', 'settings', 'pinnedPeerCertSha256']} noStyle>
            <Select
              mode="tags"
              tokenSeparators={[',', ' ']}
              placeholder={t('xray.hexSha256')}
              style={{ width: 'calc(100% - 64px)' }}
            />
          </Form.Item>
          <Button
            aria-label={t('xray.pinFromCertificate')}
            icon={<FileProtectOutlined />}
            loading={pinLoading === 'local'}
            onClick={pinFromCertificate}
            title={t('xray.pinFromCertificate')}
          />
          <Button
            aria-label={t('xray.pinFromRemote')}
            icon={<CloudDownloadOutlined />}
            loading={pinLoading === 'remote'}
            onClick={pinFromRemote}
            title={t('xray.pinFromRemote')}
          />
        </Space.Compact>
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'tlsSettings', 'settings', 'verifyPeerCertByName']}
        label={t('xray.verifyPeerByName')}
        tooltip={t('xray.verifyPeerByNameHelp')}
      >
        <Input placeholder="example.com" />
      </Form.Item>
    </>
  );
}

export function XrayOutboundTlsFields() {
  const { t } = useI18n();
  const utlsOptions = getUtlsOptions(t);
  return (
    <>
      <Form.Item label="SNI" name={['streamSettings', 'tlsSettings', 'serverName']}>
        <Input placeholder={t('xray.serverName')} />
      </Form.Item>
      <Form.Item label="uTLS" name={['streamSettings', 'tlsSettings', 'fingerprint']}>
        <Select allowClear options={utlsOptions} />
      </Form.Item>
      <Form.Item label="ALPN" name={['streamSettings', 'tlsSettings', 'alpn']}>
        <Select mode="multiple" tokenSeparators={[',']} options={alpnOptions} />
      </Form.Item>
      <Form.Item label="ECH" name={['streamSettings', 'tlsSettings', 'echConfigList']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.verifyPeerName')} name={['streamSettings', 'tlsSettings', 'verifyPeerCertByName']}>
        <Input placeholder="cloudflare-dns.com" />
      </Form.Item>
      <Form.Item label="Pinned SHA256" name={['streamSettings', 'tlsSettings', 'pinnedPeerCertSha256']}>
        <Input placeholder="base64 SHA256" />
      </Form.Item>
    </>
  );
}
