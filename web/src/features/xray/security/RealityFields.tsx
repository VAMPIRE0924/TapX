import { useState } from 'react';
import { RadarChartOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import {
  Alert,
  Button,
  Collapse,
  Descriptions,
  Divider,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  type FormInstance,
  type TableColumnsType,
} from 'antd';
import { getUtlsOptions } from '../options';
import { useI18n } from '../../../i18n/I18nProvider';
import { randomLowerAndNumber, randomShortIds } from '../../../shared/random';
import { getPanelObject, postPanelResult } from './api';

interface RealityScanResult {
  target: string;
  feasible: boolean;
  reason?: string;
  ip?: string;
  tlsVersion?: string;
  alpn?: string;
  curveID?: string;
  certValid?: boolean;
  certSubject?: string;
  certIssuer?: string;
  latencyMs?: number;
  serverNames?: string[];
}

export function XrayInboundRealityFields({ form }: { form: FormInstance }) {
  const { t } = useI18n();
  const utlsOptions = getUtlsOptions(t);
  const [scanning, setScanning] = useState(false);
  const [scanResult, setScanResult] = useState<RealityScanResult | null>(null);
  const [scannerOpen, setScannerOpen] = useState(false);
  const [keyLoading, setKeyLoading] = useState(false);
  const [mldsaLoading, setMldsaLoading] = useState(false);

  function randomizeShortIds() {
    form.setFieldValue(['streamSettings', 'realitySettings', 'shortIds'], randomShortIds());
  }

  function randomizeSpiderX() {
    form.setFieldValue(['streamSettings', 'realitySettings', 'settings', 'spiderX'], `/${randomLowerAndNumber(15)}`);
  }

  async function generateKeypair() {
    setKeyLoading(true);
    try {
      const pair = await getPanelObject<{ privateKey?: string; publicKey?: string }>('/api/xray/reality/x25519');
      form.setFieldValue(['streamSettings', 'realitySettings', 'privateKey'], pair.privateKey || '');
      form.setFieldValue(['streamSettings', 'realitySettings', 'settings', 'publicKey'], pair.publicKey || '');
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setKeyLoading(false);
    }
  }

  function clearKeypair() {
    form.setFieldValue(['streamSettings', 'realitySettings', 'privateKey'], '');
    form.setFieldValue(['streamSettings', 'realitySettings', 'settings', 'publicKey'], '');
  }

  async function generateMldsa65() {
    setMldsaLoading(true);
    try {
      const result = await getPanelObject<{ seed?: string; verify?: string }>('/api/xray/reality/mldsa65');
      form.setFieldValue(['streamSettings', 'realitySettings', 'mldsa65Seed'], result.seed || '');
      form.setFieldValue(['streamSettings', 'realitySettings', 'settings', 'mldsa65Verify'], result.verify || '');
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setMldsaLoading(false);
    }
  }

  function clearMldsa65() {
    form.setFieldValue(['streamSettings', 'realitySettings', 'mldsa65Seed'], '');
    form.setFieldValue(['streamSettings', 'realitySettings', 'settings', 'mldsa65Verify'], '');
  }

  function applyScanResult(result: RealityScanResult) {
    form.setFieldValue(['streamSettings', 'realitySettings', 'target'], result.target);
    if (result.serverNames?.length) {
      form.setFieldValue(['streamSettings', 'realitySettings', 'serverNames'], result.serverNames);
    }
    setScanResult(result);
  }

  async function scanRealityTarget() {
    const target = String(form.getFieldValue(['streamSettings', 'realitySettings', 'target']) || '').trim();
    const error = validateRealityTarget(target, t);
    if (error) {
      message.warning(error);
      return;
    }
    setScanning(true);
    try {
      const result = await postPanelResult<RealityScanResult>('/api/xray/reality/scan', { target });
      applyScanResult(result);
      if (result.feasible) {
        message.success(t('xray.realityTargetUsable'));
      } else {
        message.warning(result.reason || t('xray.realityTargetUnsuitable'));
      }
    } catch (err) {
      const result = {
        target,
        feasible: false,
        reason: t('xray.targetScanFailed', { error: err instanceof Error ? err.message : String(err) }),
      };
      setScanResult(result);
    } finally {
      setScanning(false);
    }
  }

  async function scanRealityCandidates(targets?: string): Promise<RealityScanResult[]> {
    try {
      return await postPanelResult<RealityScanResult[]>('/api/xray/reality/scan-many', targets ? { targets } : {});
    } catch (error) {
      message.warning(t('xray.targetScanFailed', { error: error instanceof Error ? error.message : String(error) }));
      return [];
    }
  }

  return (
    <>
      <Form.Item name={['streamSettings', 'realitySettings', 'show']} label={t('xray.show')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'xver']} label="Xver">
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'settings', 'fingerprint']} label="uTLS">
        <Select options={utlsOptions.filter((option) => option.value)} />
      </Form.Item>
      <Form.Item label={t('xray.target')} tooltip={t('xray.realityTargetHelp')}>
        <Space.Compact block style={{ display: 'flex' }}>
          <Form.Item
            name={['streamSettings', 'realitySettings', 'target']}
            noStyle
            rules={[
              {
                validator: async (_, value) => {
                  const error = validateRealityTarget(typeof value === 'string' ? value : '', t);
                  if (error) throw new Error(error);
                },
              },
            ]}
          >
            <Input style={{ flex: 1 }} placeholder="example.com:443" />
          </Form.Item>
          <Button icon={<RadarChartOutlined />} loading={scanning} onClick={scanRealityTarget}>
            {t('xray.scan')}
          </Button>
          <Button icon={<SearchOutlined />} onClick={() => setScannerOpen(true)}>
            {t('xray.findTarget')}
          </Button>
        </Space.Compact>
      </Form.Item>
      {scanResult ? (
        <Form.Item label=" " colon={false}>
          <Alert
            type={scanResult.feasible ? 'success' : 'warning'}
            showIcon
            title={scanResult.feasible ? t('xray.realityTargetUsable') : scanResult.reason || t('xray.realityTargetUnsuitable')}
            description={
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="TLS">{scanResult.tlsVersion || '-'}</Descriptions.Item>
                <Descriptions.Item label="ALPN">{scanResult.alpn || '-'}</Descriptions.Item>
                <Descriptions.Item label={t('xray.curve')}>{scanResult.curveID || '-'}</Descriptions.Item>
                <Descriptions.Item label={t('xray.certificate')}>
                  {scanResult.certValid
                    ? `${scanResult.certSubject || '-'} (${scanResult.certIssuer || '-'})`
                    : t('xray.invalidCertificate')}
                </Descriptions.Item>
                <Descriptions.Item label={t('xray.latency')}>
                  {scanResult.latencyMs && scanResult.latencyMs > 0 ? `${scanResult.latencyMs} ms` : '-'}
                </Descriptions.Item>
              </Descriptions>
            }
          />
        </Form.Item>
      ) : null}
      <Form.Item label="SNI" name={['streamSettings', 'realitySettings', 'serverNames']}>
        <Select mode="tags" tokenSeparators={[',']} style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'maxTimediff']} label={t('xray.maxTimeDifference')}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'minClientVer']} label={t('xray.minClientVersion')}>
        <Input placeholder="25.9.11" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'maxClientVer']} label={t('xray.maxClientVersion')}>
        <Input placeholder="25.9.11" />
      </Form.Item>
      <Form.Item label="Short IDs">
        <Space.Compact block style={{ display: 'flex' }}>
          <Form.Item name={['streamSettings', 'realitySettings', 'shortIds']} noStyle>
            <Select mode="tags" tokenSeparators={[',']} style={{ flex: 1 }} />
          </Form.Item>
          <Button aria-label={t('xray.regenerate')} icon={<ReloadOutlined />} onClick={randomizeShortIds} />
        </Space.Compact>
      </Form.Item>
      <Form.Item label="SpiderX" tooltip={t('xray.spiderXHelp')}>
        <Space.Compact block style={{ display: 'flex' }}>
          <Form.Item name={['streamSettings', 'realitySettings', 'settings', 'spiderX']} noStyle>
            <Input style={{ flex: 1 }} />
          </Form.Item>
          <Button aria-label={t('xray.regenerate')} icon={<ReloadOutlined />} onClick={randomizeSpiderX} />
        </Space.Compact>
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'settings', 'publicKey']} label={t('xray.publicKey')}>
        <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'privateKey']} label={t('xray.privateKey')}>
        <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} />
      </Form.Item>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={keyLoading} onClick={generateKeypair}>
            {t('xray.generateKeypair')}
          </Button>
          <Button danger onClick={clearKeypair}>
            {t('xray.clear')}
          </Button>
        </Space>
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'mldsa65Seed']} label="mldsa65 Seed">
        <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'settings', 'mldsa65Verify']} label="mldsa65 Verify">
        <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
      </Form.Item>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={mldsaLoading} onClick={generateMldsa65}>
            {t('xray.getNewSeed')}
          </Button>
          <Button danger onClick={clearMldsa65}>
            {t('xray.clear')}
          </Button>
        </Space>
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'masterKeyLog']}
        label={t('xray.masterKeyLog')}
        tooltip={t('xray.masterKeyLogHelp')}
      >
        <Input placeholder="/path/to/sslkeylog.txt" />
      </Form.Item>
      <Collapse
        style={{ marginBottom: 14 }}
        items={[
          {
            key: 'limitFallback',
            label: 'Limit Fallback',
            children: (
              <>
                {(['limitFallbackUpload', 'limitFallbackDownload'] as const).map((direction) => (
                  <div key={direction}>
                    <Divider style={{ margin: '0 0 14px 0' }}>
                      {direction === 'limitFallbackUpload' ? t('xray.uploadLimitFallback') : t('xray.downloadLimitFallback')}
                    </Divider>
                    <Form.Item
                      name={['streamSettings', 'realitySettings', direction, 'afterBytes']}
                      label="After Bytes"
                      tooltip={t('xray.afterBytesHelp')}
                    >
                      <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item
                      name={['streamSettings', 'realitySettings', direction, 'bytesPerSec']}
                      label="Bytes Per Sec"
                      tooltip={t('xray.bytesPerSecHelp')}
                    >
                      <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item
                      name={['streamSettings', 'realitySettings', direction, 'burstBytesPerSec']}
                      label="Burst Bytes Per Sec"
                      tooltip={t('xray.burstBytesPerSecHelp')}
                    >
                      <InputNumber min={0} />
                    </Form.Item>
                  </div>
                ))}
              </>
            ),
          },
        ]}
      />
      <RealityTargetScannerModal
        open={scannerOpen}
        onClose={() => setScannerOpen(false)}
        scanRealityCandidates={scanRealityCandidates}
        onPick={applyScanResult}
      />
    </>
  );
}

export function XrayOutboundRealityFields() {
  const { t } = useI18n();
  const utlsOptions = getUtlsOptions(t);
  return (
    <>
      <Form.Item label="SNI" name={['streamSettings', 'realitySettings', 'serverName']}>
        <Input />
      </Form.Item>
      <Form.Item label="uTLS" name={['streamSettings', 'realitySettings', 'fingerprint']}>
        <Select options={utlsOptions.filter((option) => option.value)} />
      </Form.Item>
      <Form.Item label="Short ID" name={['streamSettings', 'realitySettings', 'shortId']}>
        <Input />
      </Form.Item>
      <Form.Item label="SpiderX" name={['streamSettings', 'realitySettings', 'spiderX']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.publicKey')} name={['streamSettings', 'realitySettings', 'publicKey']}>
        <Input.TextArea autoSize={{ minRows: 2 }} />
      </Form.Item>
      <Form.Item label="mldsa65 Verify" name={['streamSettings', 'realitySettings', 'mldsa65Verify']}>
        <Input.TextArea autoSize={{ minRows: 2 }} />
      </Form.Item>
    </>
  );
}

function RealityTargetScannerModal({
  open,
  onClose,
  scanRealityCandidates,
  onPick,
}: {
  open: boolean;
  onClose: () => void;
  scanRealityCandidates: (targets?: string) => Promise<RealityScanResult[]>;
  onPick: (result: RealityScanResult) => void;
}) {
  const { t } = useI18n();
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<RealityScanResult[]>([]);

  async function runScan(targets?: string) {
    setLoading(true);
    try {
      setResults(await scanRealityCandidates(targets));
    } finally {
      setLoading(false);
    }
  }

  const columns: TableColumnsType<RealityScanResult> = [
    {
      title: t('xray.target'),
      dataIndex: 'target',
      key: 'target',
      width: 200,
      render: (target: string, row) => (
        <Tooltip title={row.ip ? `${target} - ${row.ip}` : target}>
          <div style={{ lineHeight: 1.25 }}>
            <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{target}</div>
            {row.ip ? <div style={{ color: '#999', fontSize: 12 }}>{row.ip}</div> : null}
          </div>
        </Tooltip>
      ),
    },
    {
      title: t('xray.status'),
      dataIndex: 'feasible',
      key: 'feasible',
      width: 95,
      render: (feasible: boolean, row) =>
        feasible ? (
          <Tag color="success">{t('xray.available')}</Tag>
        ) : (
          <Tooltip title={row.reason}>
            <Tag color="warning">{t('xray.unavailable')}</Tag>
          </Tooltip>
        ),
    },
    { title: 'TLS', dataIndex: 'tlsVersion', key: 'tlsVersion', width: 60, render: (value: string) => value || '-' },
    { title: 'ALPN', dataIndex: 'alpn', key: 'alpn', width: 75, render: (value: string) => value || '-' },
    { title: t('xray.curve'), dataIndex: 'curveID', key: 'curveID', width: 130, render: (value: string) => value || '-' },
    {
      title: t('xray.certificate'),
      dataIndex: 'certSubject',
      key: 'certSubject',
      width: 160,
      ellipsis: true,
      render: (_: string, row) =>
        row.certValid ? (
          <Tooltip title={`${row.certSubject || '-'} (${row.certIssuer || '-'})`}>
            <span>{row.certSubject || '-'}</span>
          </Tooltip>
        ) : (
          <Tag>{t('xray.invalidCertificateShort')}</Tag>
        ),
    },
    {
      title: t('xray.latency'),
      dataIndex: 'latencyMs',
      key: 'latencyMs',
      width: 85,
      render: (value: number) => (value > 0 ? `${value} ms` : '-'),
    },
    {
      title: '',
      key: 'action',
      width: 64,
      render: (_, row) => (
        <Button
          type="link"
          size="small"
          onClick={() => {
            onPick(row);
            onClose();
          }}
        >
          {t('xray.use')}
        </Button>
      ),
    },
  ];

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={[
        <Button key="rescan" onClick={() => runScan(query.trim() || undefined)} loading={loading}>
          {t('xray.rescan')}
        </Button>,
        <Button key="close" type="primary" onClick={onClose}>
          {t('common.close')}
        </Button>,
      ]}
      title={t('xray.findRealityTarget')}
      width={960}
    >
      <Space orientation="vertical" size="small" style={{ width: '100%' }}>
        <Form.Item label={t('xray.target')} tooltip={t('xray.findRealityTargetHelp')} style={{ marginBottom: 0 }}>
          <Input.Search
            allowClear
            enterButton={t('xray.scan')}
            loading={loading}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onSearch={() => runScan(query.trim() || undefined)}
            placeholder="example.com:443, cloudflare.com:443"
          />
        </Form.Item>
        <Table<RealityScanResult>
          size="small"
          rowKey="target"
          loading={loading}
          columns={columns}
          dataSource={results}
          pagination={false}
          scroll={{ y: 360 }}
        />
      </Space>
    </Modal>
  );
}

function validateRealityTarget(target: string, t: ReturnType<typeof useI18n>['t']): string | undefined {
  const trimmed = target.trim();
  if (!trimmed) return t('xray.targetRequired');
  if (trimmed.startsWith('/') || trimmed.startsWith('@')) return undefined;
  if (/^\d+$/.test(trimmed)) {
    const port = Number(trimmed);
    if (port >= 1 && port <= 65535) return undefined;
    return t('xray.portRange');
  }
  const lastColon = trimmed.lastIndexOf(':');
  if (lastColon <= 0 || lastColon === trimmed.length - 1) return t('xray.targetPortRequired');
  const portPart = trimmed.slice(lastColon + 1);
  if (!/^\d+$/.test(portPart)) return t('xray.portNumeric');
  const port = Number(portPart);
  if (port < 1 || port > 65535) return t('xray.portRange');
  return undefined;
}
