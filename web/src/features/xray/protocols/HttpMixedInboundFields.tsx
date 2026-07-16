import { Button, Form, Input, Select, Space, Switch } from 'antd';
import { MinusOutlined, PlusOutlined } from '@ant-design/icons';
import { InputAddon } from '../../../components/InputAddon';
import { useI18n } from '../../../i18n/I18nProvider';
import { randomLowerAndNumber } from '../../../shared/random';

export function HttpInboundFields() {
  const { t } = useI18n();
  return (
    <>
      <AccountsList />
      <Form.Item name={['settings', 'allowTransparent']} label={t('xray.allowTransparent')} valuePropName="checked">
        <Switch />
      </Form.Item>
    </>
  );
}

export function MixedInboundFields({ mixedUdpOn }: { mixedUdpOn: boolean }) {
  const { t } = useI18n();
  return (
    <>
      <AccountsList />
      <Form.Item name={['settings', 'auth']} label={t('xray.authentication')}>
        <Select
          options={[
            { value: 'noauth', label: 'noauth' },
            { value: 'password', label: 'password' },
          ]}
        />
      </Form.Item>
      <Form.Item name={['settings', 'udp']} label="UDP" valuePropName="checked">
        <Switch />
      </Form.Item>
      {mixedUdpOn ? (
        <Form.Item name={['settings', 'ip']} label="UDP IP">
          <Input />
        </Form.Item>
      ) : null}
    </>
  );
}

function AccountsList() {
  const { t } = useI18n();
  return (
    <Form.List name={['settings', 'accounts']}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label={t('xray.accounts')}>
            <Button
              size="small"
              onClick={() => add({ user: randomLowerAndNumber(8), pass: randomLowerAndNumber(12) })}
            >
              <PlusOutlined /> {t('xray.add')}
            </Button>
          </Form.Item>
          {fields.length > 0 ? (
            <Form.Item wrapperCol={{ span: 24 }}>
              {fields.map((field, index) => (
                <Space.Compact key={field.key} block style={{ marginBottom: 8 }}>
                  <InputAddon>{String(index + 1)}</InputAddon>
                  <Form.Item name={[field.name, 'user']} noStyle>
                    <Input placeholder={t('xray.username')} />
                  </Form.Item>
                  <Form.Item name={[field.name, 'pass']} noStyle>
                    <Input placeholder={t('xray.password')} />
                  </Form.Item>
                  <Button aria-label={t('xray.remove')} onClick={() => remove(field.name)}>
                    <MinusOutlined />
                  </Button>
                </Space.Compact>
              ))}
            </Form.Item>
          ) : null}
        </>
      )}
    </Form.List>
  );
}
