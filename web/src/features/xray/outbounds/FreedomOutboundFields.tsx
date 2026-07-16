import { AutoComplete, Button, Form, Input, InputNumber, Select, Switch, type FormInstance } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useI18n } from '../../../i18n/I18nProvider';

const domainStrategyOptions = [
  'AsIs',
  'UseIP',
  'UseIPv4',
  'UseIPv6',
  'UseIPv6v4',
  'UseIPv4v6',
  'ForceIP',
  'ForceIPv6v4',
  'ForceIPv6',
  'ForceIPv4v6',
  'ForceIPv4',
].map((value) => ({ value, label: value }));

const networkOptions = ['tcp', 'udp', 'tcp,udp'].map((value) => ({ value, label: value }));
const noiseTypeOptions = ['rand', 'base64', 'str', 'hex'].map((value) => ({ value, label: value }));
const applyToOptions = ['ip', 'ipv4', 'ipv6'].map((value) => ({ value, label: value }));
const actionOptions = ['allow', 'block'].map((value) => ({ value, label: value }));

export function FreedomOutboundFields({ form }: { form: FormInstance }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.strategy')} name={['settings', 'domainStrategy']}>
        <Select options={[{ value: '', label: t('xray.noneWrapped') }, ...domainStrategyOptions]} />
      </Form.Item>
      <Form.Item label={t('xray.redirect')} name={['settings', 'redirect']}>
        <Input />
      </Form.Item>
      <Form.Item label={t('xray.userLevel')} name={['settings', 'userLevel']}>
        <InputNumber min={0} style={{ width: '100%' }} />
      </Form.Item>
      <Form.Item label="Proxy protocol" name={['settings', 'proxyProtocol']}>
        <Select
          options={[
            { value: 0, label: t('xray.noneWrapped') },
            { value: 1, label: 'v1' },
            { value: 2, label: 'v2' },
          ]}
        />
      </Form.Item>
      <FragmentFields form={form} />
      <NoisesFields />
      <FinalRulesFields form={form} />
    </>
  );
}

function FragmentFields({ form }: { form: FormInstance }) {
  const { t } = useI18n();
  return (
    <Form.Item shouldUpdate noStyle>
      {() => {
        const fragment = (form.getFieldValue(['settings', 'fragment']) || {}) as {
          packets?: string;
          length?: string;
          interval?: string;
          maxSplit?: string;
        };
        const enabled = !!(fragment.length || fragment.interval || fragment.maxSplit);
        return (
          <>
            <Form.Item label="Fragment">
              <Switch
                checked={enabled}
                onChange={(checked) => {
                  form.setFieldValue(
                    ['settings', 'fragment'],
                    checked
                      ? { packets: 'tlshello', length: '100-200', interval: '10-20', maxSplit: '300-400' }
                      : { packets: '', length: '', interval: '', maxSplit: '' },
                  );
                }}
              />
            </Form.Item>
            {enabled ? (
              <>
                <Form.Item
                  label={t('xray.packet')}
                  name={['settings', 'fragment', 'packets']}
                  rules={[
                    {
                      validator: (_rule, value) => {
                        const str = String(value ?? '').trim();
                        if (str === '' || str === 'tlshello' || /^\d+-\d+$/.test(str)) {
                          return Promise.resolve();
                        }
                        return Promise.reject(new Error('Use "tlshello" or a packet range like 1-3'));
                      },
                    },
                  ]}
                >
                  <AutoComplete
                    options={[
                      { value: 'tlshello', label: 'tlshello' },
                      { value: '1-3', label: '1-3' },
                      { value: '1-5', label: '1-5' },
                    ]}
                    placeholder="tlshello / 1-3"
                  />
                </Form.Item>
                <Form.Item label={t('xray.length')} name={['settings', 'fragment', 'length']}>
                  <Input />
                </Form.Item>
                <Form.Item label={t('xray.interval')} name={['settings', 'fragment', 'interval']}>
                  <Input />
                </Form.Item>
                <Form.Item label={t('xray.maxSplit')} name={['settings', 'fragment', 'maxSplit']}>
                  <Input />
                </Form.Item>
              </>
            ) : null}
          </>
        );
      }}
    </Form.Item>
  );
}

function NoisesFields() {
  const { t } = useI18n();
  return (
    <Form.List name={['settings', 'noises']}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label={t('xray.noise')}>
            <Switch
              checked={fields.length > 0}
              onChange={(checked) => {
                if (checked) {
                  add({ type: 'rand', packet: '10-20', delay: '10-16', applyTo: 'ip' });
                  return;
                }
                for (let index = fields.length - 1; index >= 0; index--) {
                  remove(fields[index].name);
                }
              }}
            />
            {fields.length > 0 ? (
              <Button
                size="small"
                type="primary"
                className="ml-8"
                icon={<PlusOutlined />}
                aria-label={t('xray.add')}
                onClick={() => add({ type: 'rand', packet: '10-20', delay: '10-16', applyTo: 'ip' })}
              />
            ) : null}
          </Form.Item>
          {fields.map((field, index) => (
            <div key={field.key}>
              <Form.Item wrapperCol={{ md: { span: 14, offset: 8 } }}>
                <div className="item-heading">
                  <span>{t('xray.noiseIndex', { index: index + 1 })}</span>
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
                <Form.Item label={t('xray.type')} name={[field.name, 'type']}>
                <Select options={noiseTypeOptions} />
              </Form.Item>
                <Form.Item label={t('xray.packet')} name={[field.name, 'packet']}>
                <Input />
              </Form.Item>
                <Form.Item label={t('xray.delayMs')} name={[field.name, 'delay']}>
                <Input />
              </Form.Item>
                <Form.Item label={t('xray.applyTo')} name={[field.name, 'applyTo']}>
                <Select options={applyToOptions} />
              </Form.Item>
            </div>
          ))}
        </>
      )}
    </Form.List>
  );
}

function FinalRulesFields({ form }: { form: FormInstance }) {
  const { t } = useI18n();
  return (
    <Form.List name={['settings', 'finalRules']}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label={t('xray.finalRules')}>
            <Button
              size="small"
              type="primary"
              icon={<PlusOutlined />}
              aria-label={t('xray.add')}
              onClick={() => add({ action: 'allow', network: '', port: '', ip: [], blockDelay: '' })}
            />
            <span className="ml-8" style={{ opacity: 0.6 }}>
              {t('xray.overridePrivateIpBlock')}
            </span>
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
                <Select options={actionOptions} />
              </Form.Item>
              <Form.Item label={t('xray.network')} name={[field.name, 'network']}>
                <Select allowClear placeholder="(any)" options={networkOptions} />
              </Form.Item>
              <Form.Item label={t('xray.port')} name={[field.name, 'port']}>
                <Input placeholder="80,443 / 1000-2000" />
              </Form.Item>
              <Form.Item label="IP / CIDR / geoip" name={[field.name, 'ip']}>
                <Select mode="tags" tokenSeparators={[',', ' ']} placeholder="10.0.0.0/8, geoip:private" />
              </Form.Item>
              <Form.Item shouldUpdate noStyle>
                {() => {
                  const action = form.getFieldValue(['settings', 'finalRules', field.name, 'action']);
                  if (action !== 'block') return null;
                  return (
                    <Form.Item label={t('xray.blockDelayMs')} name={[field.name, 'blockDelay']}>
                      <Input placeholder="5000-10000" />
                    </Form.Item>
                  );
                }}
              </Form.Item>
            </div>
          ))}
        </>
      )}
    </Form.List>
  );
}
