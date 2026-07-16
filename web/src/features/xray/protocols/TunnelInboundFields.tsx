import { Form, Input, InputNumber, Select, Switch } from 'antd';
import { HeaderMapEditor } from '../../../components/HeaderMapEditor';
import { useI18n } from '../../../i18n/I18nProvider';

export function TunnelInboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['settings', 'rewriteAddress']} label={t('xray.rewriteAddress')}>
        <Input />
      </Form.Item>
      <Form.Item name={['settings', 'rewritePort']} label={t('xray.rewritePort')}>
        <InputNumber min={0} max={65535} />
      </Form.Item>
      <Form.Item name={['settings', 'allowedNetwork']} label={t('xray.allowedNetwork')}>
        <Select
          options={[
            { value: 'tcp,udp', label: 'TCP, UDP' },
            { value: 'tcp', label: 'TCP' },
            { value: 'udp', label: 'UDP' },
          ]}
        />
      </Form.Item>
      <Form.Item label={t('xray.portMap')} name={['settings', 'portMap']}>
        <HeaderMapEditor mode="v1" />
      </Form.Item>
      <Form.Item name={['settings', 'followRedirect']} label={t('xray.followRedirect')} valuePropName="checked">
        <Switch />
      </Form.Item>
    </>
  );
}
