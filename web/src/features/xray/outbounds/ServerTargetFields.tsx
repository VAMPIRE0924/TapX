import { Form, Input, InputNumber } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

export function ServerTargetFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.address')} name={['settings', 'address']} rules={[{ required: true, message: t('xray.addressRequired') }]}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.port')} name={['settings', 'port']} rules={[{ required: true, message: t('xray.portRequired') }]}>
        <InputNumber min={1} max={65535} style={{ width: '100%' }} />
      </Form.Item>
    </>
  );
}
