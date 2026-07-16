import { Form, Input, Select } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

const securityOptions = ['aes-128-gcm', 'chacha20-poly1305', 'auto', 'none', 'zero'].map((value) => ({
  value,
  label: value,
}));

export function VmessOutboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label="ID" name={['settings', 'id']}>
        <Input placeholder="UUID" />
      </Form.Item>
      <Form.Item label={t('xray.security')} name={['settings', 'security']}>
        <Select options={securityOptions} />
      </Form.Item>
    </>
  );
}
