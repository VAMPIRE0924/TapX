import { Button, Form, Input, InputNumber, Select, Space, Switch, type FormInstance } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useI18n } from '../../../i18n/I18nProvider';
import { randomHex } from '../../../shared/random';

export function MtprotoInboundFields({
  form,
  routeThroughXray,
  outboundTags,
}: {
  form: FormInstance;
  routeThroughXray: boolean;
  outboundTags: string[];
}) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['settings', 'fakeTlsDomain']} label={t('xray.fakeTlsDomain')}>
        <Input
          placeholder="www.cloudflare.com"
          onChange={(event) => {
            const current = String(form.getFieldValue(['settings', 'secret']) || '');
            form.setFieldValue(['settings', 'secret'], mtprotoSecretForDomain(current, event.target.value));
          }}
        />
      </Form.Item>
      <Form.Item label={t('xray.mtprotoSecret')}>
        <Space.Compact block>
          <Form.Item name={['settings', 'secret']} noStyle>
            <Input readOnly style={{ width: 'calc(100% - 32px)' }} />
          </Form.Item>
          <Button
            aria-label={t('xray.regenerate')}
            icon={<ReloadOutlined />}
            onClick={() => {
              const domain = String(form.getFieldValue(['settings', 'fakeTlsDomain']) || '');
              form.setFieldValue(['settings', 'secret'], generateMtprotoSecret(domain));
            }}
          />
        </Space.Compact>
      </Form.Item>
      <Form.Item name={['settings', 'domainFronting', 'ip']} label={t('xray.domainFrontingIp')} tooltip={t('xray.domainFrontingHelp')}>
        <Input placeholder="127.0.0.1" />
      </Form.Item>
      <Form.Item name={['settings', 'domainFronting', 'port']} label={t('xray.domainFrontingPort')}>
        <InputNumber min={0} max={65535} placeholder="443" style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item name={['settings', 'domainFronting', 'proxyProtocol']} label={t('xray.domainFrontingProxyProtocol')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name={['settings', 'proxyProtocolListener']} label={t('xray.proxyProtocolListener')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item name={['settings', 'preferIp']} label={t('xray.preferIp')}>
        <Select
          allowClear
          placeholder="prefer-ipv6"
          options={['prefer-ipv6', 'prefer-ipv4', 'only-ipv6', 'only-ipv4'].map((value) => ({ value, label: value }))}
        />
      </Form.Item>
      <Form.Item name={['settings', 'debug']} label={t('xray.debug')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item
        name={['settings', 'routeThroughXray']}
        label={t('xray.routeThroughXray')}
        tooltip={t('xray.routeThroughXrayHelp')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      {routeThroughXray ? (
        <Form.Item
          name={['settings', 'outboundTag']}
          label={t('xray.routeOutbound')}
          tooltip={t('xray.routeOutboundHelp')}
        >
          <Select
            allowClear
            showSearch
            placeholder={t('xray.selectXrayConnector')}
            options={mtprotoOutboundTagOptions(outboundTags)}
            notFoundContent={t('xray.noXrayConnector')}
          />
        </Form.Item>
      ) : null}
    </>
  );
}

export function mtprotoOutboundTagOptions(tags: string[]): Array<{ value: string; label: string }> {
  return [...new Set(tags.map((tag) => tag.trim()).filter(Boolean))].map((tag) => ({ value: tag, label: tag }));
}

function generateMtprotoSecret(domain: string): string {
  return `ee${randomHex(32)}${domainToHex(domain)}`;
}

function mtprotoSecretForDomain(currentSecret: string, domain: string): string {
  let body = currentSecret;
  if (body.startsWith('ee') || body.startsWith('dd')) {
    body = body.slice(2);
  }
  const middle = /^[0-9a-f]{32}/i.test(body) ? body.slice(0, 32) : randomHex(32);
  return `ee${middle}${domainToHex(domain)}`;
}

function domainToHex(domain: string): string {
  return Array.from(new TextEncoder().encode(domain))
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('');
}
