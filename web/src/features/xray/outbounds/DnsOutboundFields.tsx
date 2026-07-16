import { Button, Form, Input, InputNumber, Select } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useI18n } from '../../../i18n/I18nProvider';

const dnsRuleActions = ['direct', 'drop', 'return', 'hijack'].map((value) => ({ value, label: value }));

export function DnsOutboundFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.rewriteNetwork')} name={['settings', 'rewriteNetwork']}>
        <Select
          allowClear
          placeholder={t('xray.noChange')}
          options={[
            { value: 'udp', label: 'udp' },
            { value: 'tcp', label: 'tcp' },
          ]}
        />
      </Form.Item>
      <Form.Item label={t('xray.rewriteAddress')} name={['settings', 'rewriteAddress']}>
        <Input placeholder={t('xray.keepOriginalAddress')} />
      </Form.Item>
      <Form.Item label={t('xray.rewritePort')} name={['settings', 'rewritePort']}>
        <InputNumber min={0} max={65535} style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item label={t('xray.userLevel')} name={['settings', 'userLevel']}>
        <InputNumber min={0} style={{ width: '100%' }} />
      </Form.Item>
      <Form.List name={['settings', 'rules']}>
        {(fields, { add, remove }) => (
          <>
            <Form.Item label={t('xray.rules')}>
              <Button
                size="small"
                type="primary"
                icon={<PlusOutlined />}
                aria-label={t('xray.add')}
                onClick={() => add({ action: 'direct', qType: '', domain: '', rCode: 0 })}
              />
            </Form.Item>
            {fields.map((field, index) => (
              <div key={field.key}>
                <Form.Item wrapperCol={{ md: { span: 14, offset: 8 } }}>
                  <div className="item-heading">
                    <span>{t('xray.ruleIndex', { index: index + 1 })}</span>
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
                  </div>
                </Form.Item>
                <Form.Item label={t('xray.action')} name={[field.name, 'action']}>
                  <Select options={dnsRuleActions} />
                </Form.Item>
                <Form.Item label="QType" name={[field.name, 'qType']}>
                  <Input placeholder="1,3,23-24" />
                </Form.Item>
                <Form.Item label={t('xray.domain')} name={[field.name, 'domain']}>
                  <Input placeholder="domain:example.com" />
                </Form.Item>
                <Form.Item label="RCode" name={[field.name, 'rCode']}>
                  <InputNumber min={0} max={65535} style={{ width: '100%' }} />
                </Form.Item>
              </div>
            ))}
          </>
        )}
      </Form.List>
    </>
  );
}
