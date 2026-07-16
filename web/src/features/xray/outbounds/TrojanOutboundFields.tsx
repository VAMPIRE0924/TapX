import { Form, Input } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

export function TrojanOutboundFields() {
  const { t } = useI18n();
  return (
    <Form.Item label={t('xray.password')} name={['settings', 'password']}>
      <Input />
    </Form.Item>
  );
}
