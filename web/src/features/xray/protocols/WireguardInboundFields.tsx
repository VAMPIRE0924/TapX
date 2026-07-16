import { Button, Form, Input, InputNumber, Select, Space, Switch, type FormInstance } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { generateWireguardKeypair, wireguardPublicKeyFromPrivate } from '../../../shared/wireguard';
import { useI18n } from '../../../i18n/I18nProvider';

export function WireguardInboundFields({ form }: { form: FormInstance }) {
  const { t } = useI18n();
  const privateKey = Form.useWatch(['settings', 'secretKey'], { form, preserve: true }) as string | undefined;
  const publicKey = wireguardPublicKeyFromPrivate(privateKey || '');

  function regenerate() {
    const pair = generateWireguardKeypair();
    form.setFieldValue(['settings', 'secretKey'], pair.privateKey);
  }

  return (
    <>
      <Form.Item label={t('xray.privateKey')}>
        <Space.Compact block>
          <Form.Item name={['settings', 'secretKey']} noStyle>
            <Input style={{ width: 'calc(100% - 32px)' }} />
          </Form.Item>
          <Button aria-label={t('xray.regenerate')} icon={<ReloadOutlined />} onClick={regenerate} />
        </Space.Compact>
      </Form.Item>
      <Form.Item label={t('xray.publicKey')}>
        <Input value={publicKey} disabled />
      </Form.Item>
      <Form.Item name={['settings', 'mtu']} label="MTU">
        <InputNumber />
      </Form.Item>
      <Form.Item name={['settings', 'dns']} label="DNS">
        <Input placeholder="1.1.1.1, 1.0.0.1" />
      </Form.Item>
      <Form.Item name={['settings', 'noKernelTun']} label={t('xray.noKernelTun')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name={['settings', 'domainStrategy']} label={t('xray.domainStrategy')}>
        <Select
          allowClear
          options={['ForceIP', 'ForceIPv4', 'ForceIPv4v6', 'ForceIPv6', 'ForceIPv6v4'].map((value) => ({
            value,
            label: value,
          }))}
        />
      </Form.Item>
    </>
  );
}
