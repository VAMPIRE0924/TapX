import { Form, InputNumber, Select, Switch, type FormInstance } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

export function MuxFields({ form }: { form: FormInstance }) {
  const { t } = useI18n();
  return (
    <Form.Item shouldUpdate noStyle>
      {() => {
        const enabled = form.getFieldValue(['mux', 'enabled']) === true;
        return (
          <>
            <Form.Item label="Mux" name={['mux', 'enabled']} valuePropName="checked">
              <Switch />
            </Form.Item>
            {enabled ? (
              <>
                <Form.Item label={t('xray.concurrency')} name={['mux', 'concurrency']}>
                  <InputNumber min={-1} max={1024} />
                </Form.Item>
                <Form.Item label={t('xray.xudpConcurrency')} name={['mux', 'xudpConcurrency']}>
                  <InputNumber min={-1} max={1024} />
                </Form.Item>
                <Form.Item label="xudp UDP 443" name={['mux', 'xudpProxyUDP443']}>
                  <Select options={['reject', 'allow', 'skip'].map((value) => ({ value, label: value }))} />
                </Form.Item>
              </>
            ) : null}
          </>
        );
      }}
    </Form.Item>
  );
}
