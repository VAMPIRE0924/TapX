import { Form, Input, InputNumber, Select, Switch, type FormInstance } from 'antd';
import { HeaderMapEditor } from '../../../components/HeaderMapEditor';
import { useI18n } from '../../../i18n/I18nProvider';

const masqueradePath = ['streamSettings', 'hysteriaSettings', 'masquerade'];

export function HysteriaFields({ form, direction }: { form: FormInstance; direction: 'inbound' | 'outbound' }) {
  const { t } = useI18n();
  const masquerade = Form.useWatch(masqueradePath, { form, preserve: true }) as { type?: string } | undefined;

  return (
    <>
      <Form.Item label={t('xray.version')} name={['streamSettings', 'hysteriaSettings', 'version']}>
        <InputNumber min={2} max={2} disabled />
      </Form.Item>
      {direction === 'outbound' ? (
        <Form.Item label={t('xray.authPassword')} name={['streamSettings', 'hysteriaSettings', 'auth']} tooltip={t('xray.hysteriaAuthHelp')}>
          <Input placeholder="hysteria-auth-token" />
        </Form.Item>
      ) : null}
      <Form.Item label={t('xray.udpIdleTimeout')} name={['streamSettings', 'hysteriaSettings', 'udpIdleTimeout']} tooltip={t('xray.udpIdleHelp')}>
        <InputNumber min={2} max={600} placeholder="60" style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item label={t('xray.masquerade')} tooltip={t('xray.masqueradeHelp')}>
        <Switch
          checked={!!masquerade}
          onChange={(checked) =>
            form.setFieldValue(
              masqueradePath,
              checked
                ? {
                  type: '',
                  dir: '',
                  url: '',
                  rewriteHost: false,
                  insecure: false,
                  content: '',
                  headers: {},
                  statusCode: 0,
                }
                : undefined,
            )
          }
        />
      </Form.Item>
      {masquerade ? <HysteriaMasqueradeFields type={masquerade.type || ''} /> : null}
    </>
  );
}

function HysteriaMasqueradeFields({ type }: { type: string }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.type')} name={[...masqueradePath, 'type']}>
        <Select
          options={[
            { value: '', label: 'default (404 page)' },
            { value: 'proxy', label: 'proxy (reverse proxy)' },
            { value: 'file', label: 'file (serve directory)' },
            { value: 'string', label: 'string (fixed body)' },
          ]}
        />
      </Form.Item>
      {type === 'proxy' ? (
        <>
          <Form.Item label={t('xray.upstreamUrl')} name={[...masqueradePath, 'url']}>
            <Input placeholder="https://www.example.com" />
          </Form.Item>
          <Form.Item label={t('xray.rewriteHost')} name={[...masqueradePath, 'rewriteHost']} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label={t('xray.skipTlsVerify')} name={[...masqueradePath, 'insecure']} valuePropName="checked">
            <Switch />
          </Form.Item>
        </>
      ) : null}
      {type === 'file' ? (
        <Form.Item label={t('xray.directory')} name={[...masqueradePath, 'dir']}>
          <Input placeholder="/var/www/html" />
        </Form.Item>
      ) : null}
      {type === 'string' ? (
        <>
          <Form.Item label={t('xray.statusCode')} name={[...masqueradePath, 'statusCode']}>
            <InputNumber min={0} max={599} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label={t('xray.responseBody')} name={[...masqueradePath, 'content']}>
            <Input.TextArea autoSize={{ minRows: 3 }} />
          </Form.Item>
          <Form.Item label={t('xray.requestHeaders')} name={[...masqueradePath, 'headers']}>
            <HeaderMapEditor mode="v1" />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}
