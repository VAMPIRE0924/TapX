import type { ReactNode } from 'react';
import { Button, Form, Input, InputNumber, Space, Tooltip } from 'antd';
import { MinusOutlined, PlusOutlined } from '@ant-design/icons';
import { useI18n } from '../../../i18n/I18nProvider';

export function TunInboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['settings', 'name']} label={t('xray.interfaceName')}>
        <Input placeholder="xray0" />
      </Form.Item>
      <Form.Item name={['settings', 'mtu']} label="MTU">
        <InputNumber min={0} />
      </Form.Item>
      <ListOfInputs name={['settings', 'gateway']} label={t('xray.gateway')} placeholders={['10.0.0.1/16', 'fc00::1/64']} />
      <ListOfInputs name={['settings', 'dns']} label="DNS" placeholders={['1.1.1.1', '8.8.8.8']} />
      <Form.Item name={['settings', 'userLevel']} label={t('xray.userLevel')}>
        <InputNumber min={0} />
      </Form.Item>
      <ListOfInputs
        name={['settings', 'autoSystemRoutingTable']}
        label={<Tooltip title={t('xray.autoRouteHelp')}>{t('xray.autoRouteTable')}</Tooltip>}
        placeholders={['0.0.0.0/0', '::/0']}
      />
      <Form.Item
        name={['settings', 'autoOutboundsInterface']}
        label={<Tooltip title={t('xray.autoOutboundHelp')}>{t('xray.autoOutboundInterface')}</Tooltip>}
      >
        <Input placeholder="auto" />
      </Form.Item>
    </>
  );
}

function ListOfInputs({
  name,
  label,
  placeholders,
}: {
  name: Array<string | number>;
  label: ReactNode;
  placeholders?: string[];
}) {
  const { t } = useI18n();
  return (
    <Form.List name={name}>
      {(fields, { add, remove }) => (
        <Form.Item label={label}>
          <Button aria-label={t('xray.add')} size="small" onClick={() => add('')}>
            <PlusOutlined />
          </Button>
          {fields.map((field, index) => (
            <Space.Compact key={field.key} block style={{ marginTop: 4 }}>
              <Form.Item name={field.name} noStyle>
                <Input placeholder={placeholders?.[index] || ''} />
              </Form.Item>
              <Button aria-label={t('xray.remove')} size="small" onClick={() => remove(field.name)}>
                <MinusOutlined />
              </Button>
            </Space.Compact>
          ))}
        </Form.Item>
      )}
    </Form.List>
  );
}
