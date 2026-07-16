import { Form, Input, Switch, type FormInstance } from 'antd';
import { HeaderMapEditor } from '../../../components/HeaderMapEditor';
import type { XrayDirection } from '../XrayFormFields';
import { useI18n } from '../../../i18n/I18nProvider';

export function RawTransportFields({ form, direction }: { form: FormInstance; direction: XrayDirection }) {
  const { t } = useI18n();
  return (
    <Form.Item shouldUpdate noStyle>
      {() => {
        const headerType = String(form.getFieldValue(['streamSettings', 'tcpSettings', 'header', 'type']) || 'none');
        return (
          <>
            {direction === 'inbound' ? (
              <Form.Item name={['streamSettings', 'tcpSettings', 'acceptProxyProtocol']} label="Proxy Protocol" tooltip={t('xray.proxyProtocolHelp')} valuePropName="checked">
                <Switch />
              </Form.Item>
            ) : null}
            <Form.Item label={t('xray.httpCamouflage')} tooltip={t('xray.httpCamouflageHelp')}>
              <Switch
                checked={headerType === 'http'}
                onChange={(checked) => {
                  form.setFieldValue(
                    ['streamSettings', 'tcpSettings', 'header'],
                    checked
                      ? {
                        type: 'http',
                        request: { version: '1.1', method: 'GET', path: ['/'], headers: {} },
                        response: { version: '1.1', status: '200', reason: 'OK', headers: {} },
                      }
                      : { type: 'none' },
                  );
                }}
              />
            </Form.Item>
            {headerType === 'http' ? <HttpCamouflageFields /> : null}
          </>
        );
      }}
    </Form.Item>
  );
}

function HttpCamouflageFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.requestVersion')} name={['streamSettings', 'tcpSettings', 'header', 'request', 'version']}>
        <Input placeholder="1.1" />
      </Form.Item>
      <Form.Item label={t('xray.requestMethod')} name={['streamSettings', 'tcpSettings', 'header', 'request', 'method']}>
        <Input placeholder="GET" />
      </Form.Item>
      <Form.Item
        label={t('xray.requestPath')}
        name={['streamSettings', 'tcpSettings', 'header', 'request', 'path']}
        getValueProps={(value) => ({ value: Array.isArray(value) ? value.join(',') : value })}
        getValueFromEvent={(event) => {
          const raw = String(event?.target?.value || '');
          const parts = raw.split(',').map((item) => item.trim()).filter(Boolean);
          return parts.length ? parts : ['/'];
        }}
      >
        <Input placeholder="/" />
      </Form.Item>
      <Form.Item label={t('xray.requestHeaders')} name={['streamSettings', 'tcpSettings', 'header', 'request', 'headers']}>
        <HeaderMapEditor mode="v2" />
      </Form.Item>
      <Form.Item label={t('xray.responseVersion')} name={['streamSettings', 'tcpSettings', 'header', 'response', 'version']}>
        <Input placeholder="1.1" />
      </Form.Item>
      <Form.Item label={t('xray.responseStatus')} name={['streamSettings', 'tcpSettings', 'header', 'response', 'status']}>
        <Input placeholder="200" />
      </Form.Item>
      <Form.Item label={t('xray.responseReason')} name={['streamSettings', 'tcpSettings', 'header', 'response', 'reason']}>
        <Input placeholder="OK" />
      </Form.Item>
      <Form.Item label={t('xray.responseHeaders')} name={['streamSettings', 'tcpSettings', 'header', 'response', 'headers']}>
        <HeaderMapEditor mode="v2" />
      </Form.Item>
    </>
  );
}
