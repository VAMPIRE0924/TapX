import { Form, Input } from 'antd';
import { HeaderMapEditor } from '../../../components/HeaderMapEditor';
import { useI18n } from '../../../i18n/I18nProvider';

export function HttpOutboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.username')} name={['settings', 'user']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.password')} name={['settings', 'pass']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.requestHeaders')} name={['settings', 'headers']}>
        <HeaderMapEditor mode="v1" />
      </Form.Item>
    </>
  );
}

export function SocksOutboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.username')} name={['settings', 'user']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.password')} name={['settings', 'pass']}>
        <Input />
      </Form.Item>
    </>
  );
}
