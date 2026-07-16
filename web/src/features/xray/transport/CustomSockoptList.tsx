import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Divider, Form, Input, Select } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

const systemOptions = ['linux', 'windows', 'darwin'].map((value) => ({ value, label: value }));
const typeOptions = ['int', 'str'].map((value) => ({ value, label: value }));

export function CustomSockoptList({
  name = ['streamSettings', 'sockopt', 'customSockopt'],
}: {
  name?: Array<string | number>;
}) {
  const { t } = useI18n();
  return (
    <Form.List name={name}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label={t('xray.customSockopt')}>
            <Button
              type="dashed"
              size="small"
              icon={<PlusOutlined />}
              onClick={() => add({ type: 'int', level: '6', opt: '', value: '' })}
            >
              {t('xray.addCustomSockopt')}
            </Button>
          </Form.Item>
          {fields.map((field, index) => (
            <div key={field.key}>
              <Divider plain style={{ margin: '4px 0 8px' }}>
                {t('xray.customSockoptIndex', { index: index + 1 })}
                <Button
                  danger
                  size="small"
                  type="text"
                  icon={<DeleteOutlined />}
                  aria-label={t('common.delete')}
                  onClick={() => remove(field.name)}
                />
              </Divider>
              <Form.Item label="System" name={[field.name, 'system']}>
                <Select placeholder="all" allowClear options={systemOptions} />
              </Form.Item>
              <Form.Item label="Level" name={[field.name, 'level']}>
                <Input placeholder="6 (SOL_TCP)" />
              </Form.Item>
              <Form.Item label="Opt" name={[field.name, 'opt']}>
                <Input placeholder="19" />
              </Form.Item>
              <Form.Item label="Type" name={[field.name, 'type']}>
                <Select options={typeOptions} />
              </Form.Item>
              <Form.Item label="Value" name={[field.name, 'value']}>
                <Input placeholder="value" />
              </Form.Item>
            </div>
          ))}
        </>
      )}
    </Form.List>
  );
}
