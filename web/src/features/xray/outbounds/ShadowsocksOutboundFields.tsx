import { Form, Input, InputNumber, Select, Switch } from 'antd';
import { ssMethodOptions } from '../protocols/ShadowsocksInboundFields';
import { useI18n } from '../../../i18n/I18nProvider';

export function ShadowsocksOutboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.password')} name={['settings', 'password']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.encryption')} name={['settings', 'method']}>
        <Select options={ssMethodOptions} />
      </Form.Item>
      <Form.Item label="UDP over TCP" name={['settings', 'uot']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label="UoT Version" name={['settings', 'UoTVersion']}>
        <InputNumber min={1} max={2} />
      </Form.Item>
    </>
  );
}
