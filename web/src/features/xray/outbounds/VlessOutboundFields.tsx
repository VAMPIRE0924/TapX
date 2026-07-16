import { Form, Input } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

export function VlessOutboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label="ID" name={['settings', 'id']}>
        <Input placeholder="UUID" />
      </Form.Item>
      <Form.Item label={t('xray.encryption')} name={['settings', 'encryption']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.reverseTag')} name={['settings', 'reverseTag']}>
        <Input placeholder="reverse-tunnel-1" />
      </Form.Item>
    </>
  );
}
