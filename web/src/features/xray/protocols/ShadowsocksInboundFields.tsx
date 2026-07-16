import { Button, Form, Input, Select, Space, Switch, type FormInstance } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useI18n } from '../../../i18n/I18nProvider';
import { randomBase64 } from '../../../shared/random';

export const ssMethodOptions = [
  'aes-256-gcm',
  'chacha20-poly1305',
  'chacha20-ietf-poly1305',
  'xchacha20-ietf-poly1305',
  '2022-blake3-aes-128-gcm',
  '2022-blake3-aes-256-gcm',
  '2022-blake3-chacha20-poly1305',
].map((value) => ({ value, label: value }));

export function ShadowsocksInboundFields({ form, method }: { form: FormInstance; method?: string }) {
  const { t } = useI18n();
  const is2022 = String(method || '').startsWith('2022');

  function regeneratePassword(nextMethod?: string) {
    form.setFieldValue(['settings', 'password'], randomShadowsocksPassword(nextMethod || method));
  }

  return (
    <>
      <Form.Item name={['settings', 'method']} label={t('xray.encryptionMethod')}>
        <Select onChange={(value) => regeneratePassword(value)} options={ssMethodOptions} />
      </Form.Item>
      {is2022 ? (
        <Form.Item label={t('xray.password')}>
          <Space.Compact block>
            <Form.Item name={['settings', 'password']} noStyle>
              <Input style={{ width: 'calc(100% - 32px)' }} />
            </Form.Item>
            <Button aria-label={t('xray.regenerate')} icon={<ReloadOutlined />} onClick={() => regeneratePassword()} />
          </Space.Compact>
        </Form.Item>
      ) : null}
      <Form.Item name={['settings', 'network']} label={t('xray.network')}>
        <Select
          style={{ width: 120 }}
          options={[
            { value: 'tcp,udp', label: 'TCP, UDP' },
            { value: 'tcp', label: 'TCP' },
            { value: 'udp', label: 'UDP' },
          ]}
        />
      </Form.Item>
      <Form.Item name={['settings', 'ivCheck']} label="ivCheck" valuePropName="checked">
        <Switch />
      </Form.Item>
    </>
  );
}

function randomShadowsocksPassword(method = '2022-blake3-aes-256-gcm'): string {
  const length = method === '2022-blake3-aes-128-gcm' ? 16 : 32;
  return randomBase64(length);
}
