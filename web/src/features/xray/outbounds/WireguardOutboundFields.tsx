import { Button, Form, Input, InputNumber, Select, Space, Switch, type FormInstance } from 'antd';
import { DeleteOutlined, MinusOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { InputAddon } from '../../../components/InputAddon';
import { useI18n } from '../../../i18n/I18nProvider';
import { generateWireguardKeypair, wireguardPublicKeyFromPrivate } from '../../../shared/wireguard';

const defaultPeer = {
  publicKey: '',
  psk: '',
  allowedIPs: ['0.0.0.0/0', '::/0'],
  endpoint: '',
  keepAlive: 0,
};

export function WireguardOutboundFields({ form, peers }: { form: FormInstance; peers: unknown[] }) {
  const { t } = useI18n();
  const domainStrategyOptions = ['', 'ForceIP', 'ForceIPv4', 'ForceIPv4v6', 'ForceIPv6', 'ForceIPv6v4'].map((value) => ({ value, label: value || t('xray.noneWrapped') }));
  function regenerate() {
    const pair = generateWireguardKeypair();
    form.setFieldValue(['settings', 'secretKey'], pair.privateKey);
    form.setFieldValue(['settings', 'pubKey'], pair.publicKey);
  }

  return (
    <>
      <Form.Item label={t('xray.address')} name={['settings', 'address']} tooltip="Local WireGuard addresses, separated by commas.">
        <Input placeholder="10.0.0.1, fd00::1" />
      </Form.Item>
      <Form.Item label={t('xray.privateKey')}>
        <Space.Compact block>
          <Form.Item name={['settings', 'secretKey']} noStyle>
            <Input
              aria-label={t('xray.privateKey')}
              style={{ width: 'calc(100% - 32px)' }}
              onChange={(event) => {
                const privateKey = event.target.value;
                form.setFieldValue(['settings', 'secretKey'], privateKey);
                form.setFieldValue(['settings', 'pubKey'], wireguardPublicKeyFromPrivate(privateKey));
              }}
            />
          </Form.Item>
          <Button icon={<ReloadOutlined />} aria-label={t('xray.regenerate')} onClick={regenerate} />
        </Space.Compact>
      </Form.Item>
      <Form.Item label={t('xray.publicKey')} name={['settings', 'pubKey']}>
        <Input disabled />
      </Form.Item>
      <Form.Item label={t('xray.domainStrategy')} name={['settings', 'domainStrategy']}>
        <Select options={domainStrategyOptions} />
      </Form.Item>
      <Form.Item label="MTU" name={['settings', 'mtu']}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item label={t('xray.noKernelTun')} name={['settings', 'noKernelTun']} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item label={t('xray.reserved')} name={['settings', 'reserved']} tooltip="Three reserved bytes, separated by commas.">
        <Input placeholder="1,2,3" />
      </Form.Item>
      <Form.List name={['settings', 'peers']} initialValue={peers.length ? undefined : [defaultPeer]}>
        {(fields, { add, remove }) => (
          <>
            <Form.Item label="Peers">
              <Button
                size="small"
                type="primary"
                icon={<PlusOutlined />}
                aria-label={t('xray.add')}
                onClick={() => add(defaultPeer)}
              />
            </Form.Item>
            {fields.map((field, index) => (
              <div key={field.key}>
                <Form.Item wrapperCol={{ md: { span: 14, offset: 8 } }}>
                  <div className="item-heading">
                    <span>Peer {index + 1}</span>
                    {fields.length > 1 ? (
                      <DeleteOutlined
                        className="danger-icon"
                        role="button"
                        tabIndex={0}
                        aria-label={t('xray.remove')}
                        onClick={() => remove(field.name)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault();
                            remove(field.name);
                          }
                        }}
                      />
                    ) : null}
                  </div>
                </Form.Item>
                <Form.Item label={t('xray.endpoint')} name={[field.name, 'endpoint']}>
                  <Input />
                </Form.Item>
                <Form.Item label={t('xray.publicKey')} name={[field.name, 'publicKey']}>
                  <Input />
                </Form.Item>
                <Form.Item label="PSK" name={[field.name, 'psk']}>
                  <Input />
                </Form.Item>
                <AllowedIpsFields fieldName={field.name} />
                <Form.Item label="Keep alive" name={[field.name, 'keepAlive']}>
                  <InputNumber min={0} />
                </Form.Item>
              </div>
            ))}
          </>
        )}
      </Form.List>
    </>
  );
}

function AllowedIpsFields({ fieldName }: { fieldName: number }) {
  const { t } = useI18n();
  return (
    <Form.Item label={t('xray.allowedIps')}>
      <Form.List name={[fieldName, 'allowedIPs']}>
        {(ipFields, { add, remove }) => (
          <>
            {ipFields.map((ipField, index) => (
              <Space.Compact key={ipField.key} block style={{ marginBottom: 4 }}>
                <Form.Item noStyle name={ipField.name}>
                  <Input aria-label={t('xray.allowedIps')} />
                </Form.Item>
                {ipFields.length > 1 ? (
                  <InputAddon ariaLabel={t('xray.remove')} onClick={() => remove(index)}>
                    <MinusOutlined />
                  </InputAddon>
                ) : null}
              </Space.Compact>
            ))}
            <Button size="small" icon={<PlusOutlined />} aria-label={t('xray.add')} onClick={() => add('')} />
          </>
        )}
      </Form.List>
    </Form.Item>
  );
}
