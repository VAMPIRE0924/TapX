import { useEffect, useMemo, useState } from 'react';
import { Button, Form, Input, InputNumber, Select, Space, Typography, message, type FormInstance } from 'antd';
import { getVlessEncryptionAuths } from '../../../shared/api';
import { errorMessage } from '../../../shared/localized-error';
import { useI18n } from '../../../i18n/I18nProvider';

type VlessAuthKind =
  | 'x25519'
  | 'x25519_xorpub'
  | 'x25519_random'
  | 'mlkem768'
  | 'mlkem768_xorpub'
  | 'mlkem768_random';

const authOptions: { value: VlessAuthKind; label: string }[] = [
  { value: 'x25519', label: 'X25519 (native)' },
  { value: 'x25519_xorpub', label: 'X25519 (xorpub)' },
  { value: 'x25519_random', label: 'X25519 (random)' },
  { value: 'mlkem768', label: 'ML-KEM-768 (native)' },
  { value: 'mlkem768_xorpub', label: 'ML-KEM-768 (xorpub)' },
  { value: 'mlkem768_random', label: 'ML-KEM-768 (random)' },
];

export function VlessInboundFields({ form, network, security }: { form: FormInstance; network: string; security: string }) {
  const { t } = useI18n();
  const [authKind, setAuthKind] = useState<VlessAuthKind>('x25519');
  const [generating, setGenerating] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const encryption = Form.useWatch(['settings', 'encryption'], form) as string | undefined;
  const selectedAuth = useMemo(() => describeVlessAuth(encryption || ''), [encryption]);

  useEffect(() => {
    const detected = detectVlessAuthKind(encryption || '');
    if (detected) setAuthKind(detected);
  }, [encryption]);

  async function generateAuth() {
    setGenerating(true);
    try {
      const auths = await getVlessEncryptionAuths();
      const block = auths.find((item) => item.id === authKind || matchesAuthLabel(item.label, authKind));
      if (!block) throw new Error(t('xray.backendMissingAuth', { auth: authOptions.find((item) => item.value === authKind)?.label || authKind }));
      form.setFieldValue(['settings', 'decryption'], block.decryption);
      form.setFieldValue(['settings', 'encryption'], block.encryption);
    } catch (error) {
      messageApi.warning(errorMessage(error, t, 'xray.vlessAuthFailed'));
    } finally {
      setGenerating(false);
    }
  }

  return (
    <>
      {messageContextHolder}
      <Form.Item name={['settings', 'decryption']} label={t('xray.decryption')}>
        <Input placeholder="none" />
      </Form.Item>
      <Form.Item name={['settings', 'encryption']} label={t('xray.encryption')}>
        <Input placeholder="none" />
      </Form.Item>
      <Form.Item label={t('xray.generateKey')}>
        <Space size={8} wrap>
          <Select value={authKind} onChange={setAuthKind} options={authOptions} style={{ width: 240 }} />
          <Button type="primary" loading={generating} onClick={() => void generateAuth()}>{t('xray.generate')}</Button>
          <Button
            danger
            onClick={() => {
              form.setFieldValue(['settings', 'decryption'], 'none');
              form.setFieldValue(['settings', 'encryption'], 'none');
            }}
          >
            {t('xray.clear')}
          </Button>
        </Space>
        <Typography.Text type="secondary" className="vless-auth-state">
          {t('xray.selectedAuth', { auth: selectedAuth })}
        </Typography.Text>
      </Form.Item>
      {network === 'tcp' && (security === 'tls' || security === 'reality') ? (
        <Form.Item label={t('xray.visionTestSeed')} tooltip={t('xray.visionSeedHelp')}>
          <Space.Compact block>
            {[900, 500, 900, 256].map((value, index) => (
              <Form.Item key={index} name={['settings', 'testseed', index]} noStyle initialValue={value}>
                <InputNumber min={1} style={{ width: '25%' }} />
              </Form.Item>
            ))}
          </Space.Compact>
        </Form.Item>
      ) : null}
    </>
  );
}

function describeVlessAuth(encryption: string): string {
  if (!encryption || encryption === 'none') return 'None';
  const detected = detectVlessAuthKind(encryption);
  return authOptions.find((item) => item.value === detected)?.label || 'Custom';
}

function detectVlessAuthKind(encryption: string): VlessAuthKind | null {
  const normalized = encryption.toLowerCase().replace(/[-_\s]/g, '');
  if (!normalized || normalized === 'none') return null;
  const parts = encryption.split('.').filter(Boolean);
  if (parts.length < 4 || !normalized.includes('mlkem768x25519plus')) return null;
  const keyType = parts[parts.length - 1].length > 100 ? 'mlkem768' : 'x25519';
  if (normalized.includes('xorpub')) return `${keyType}_xorpub` as VlessAuthKind;
  if (normalized.includes('random')) return `${keyType}_random` as VlessAuthKind;
  return keyType as VlessAuthKind;
}

function matchesAuthLabel(label: string | undefined, authKind: VlessAuthKind): boolean {
  const normalized = String(label || '').toLowerCase().replace(/[-_\s]/g, '');
  const wantsMlkem = authKind.startsWith('mlkem768');
  if (wantsMlkem !== normalized.includes('mlkem768')) return false;
  if (!wantsMlkem && !normalized.includes('x25519')) return false;
  if (authKind.endsWith('_xorpub')) return normalized.includes('xorpub');
  if (authKind.endsWith('_random')) return normalized.includes('random');
  return !normalized.includes('xorpub') && !normalized.includes('random');
}
