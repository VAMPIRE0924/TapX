import { Form, Input, InputNumber, Radio, Select, Switch } from 'antd';
import { useI18n } from '../../i18n/I18nProvider';
import type { AddressAssignMode, DeviceBindMode } from './endpoint-types';

type DeviceOption = { value: string; label: string };

export function EndpointBindingFields({
  bindMode,
  linkAutoOptimize,
  addressConfigEnabled,
  addressAssignMode,
  deviceOptions,
  addressPlaceholders,
}: {
  bindMode: DeviceBindMode;
  linkAutoOptimize: boolean;
  addressConfigEnabled: boolean;
  addressAssignMode: AddressAssignMode;
  deviceOptions: DeviceOption[];
  addressPlaceholders: { ipv4: string; ipv6: string; gateway: string };
}) {
  const form = Form.useFormInstance();
  const { t } = useI18n();

  return (
    <>
      <Form.Item name={['Binding', 'DeviceBindMode']} label={t('listener.deviceBindMode')} tooltip={t('listener.deviceBindModeHelp')}>
        <Radio.Group buttonStyle="solid" onChange={(event) => {
          const mode = event.target.value as DeviceBindMode;
          form.setFieldValue(['Binding', 'AutoCreateDevice'], mode === 'autoCreate');
        }}>
          <Radio.Button value="existing">{t('listener.selectExistingDevice')}</Radio.Button>
          <Radio.Button value="autoCreate">{t('listener.autoCreateByName')}</Radio.Button>
        </Radio.Group>
      </Form.Item>
      <Form.Item name={['Binding', 'AutoCreateDevice']} hidden valuePropName="checked"><Switch /></Form.Item>
      <Form.Item name={['Binding', 'InterfaceType']} label={t('listener.interfaceType')} tooltip={t('device.typeHelp')}>
        <Radio.Group buttonStyle="solid">
          <Radio.Button value="tun">TUN</Radio.Button>
          <Radio.Button value="tap">TAP</Radio.Button>
        </Radio.Group>
      </Form.Item>
      {bindMode === 'existing' ? (
        <Form.Item name={['Binding', 'DeviceID']} label={t('listener.tunTapInterface')} tooltip={t('listener.existingDeviceHelp')}>
          <Select allowClear showSearch placeholder={t('listener.selectCreatedDevice')} options={deviceOptions} notFoundContent={t('listener.noDeviceForType')} />
        </Form.Item>
      ) : (
        <>
          <Form.Item
            name={['Binding', 'DeviceName']}
            label={t('listener.tunTapInterface')}
            rules={[{ required: true, message: t('device.interfaceNameRequired') }]}
            tooltip={t('listener.autoCreateDeviceHelp')}
          >
            <Input placeholder={t('listener.interfaceNamePlaceholder')} />
          </Form.Item>
          <Form.Item
            name={['Binding', 'LinkAutoOptimize']}
            label={t('device.linkAutoOptimize')}
            tooltip={t('device.linkAutoOptimizeHelp')}
            valuePropName="checked"
          >
            <Switch onChange={(enabled) => {
              if (enabled) form.setFieldValue(['Binding', 'MSSClamp'], 0);
            }} />
          </Form.Item>
          <Form.Item
            name={['Binding', 'MTU']}
            label={linkAutoOptimize ? t('device.mtuCeiling') : 'MTU'}
            tooltip={linkAutoOptimize ? t('device.mtuCeilingHelp') : t('device.mtuHelp')}
          >
            <InputNumber min={576} max={9000} />
          </Form.Item>
          {!linkAutoOptimize ? (
            <Form.Item name={['Binding', 'MSSClamp']} label={t('device.mssClamp')} tooltip={t('device.mssClampHelp')}>
              <InputNumber min={0} max={9000} placeholder="0" />
            </Form.Item>
          ) : null}
          <Form.Item name={['Binding', 'AddressConfigEnabled']} label={t('device.configureAddress')} tooltip={t('device.configureAddressHelp')} valuePropName="checked"><Switch /></Form.Item>
          {addressConfigEnabled ? (
            <>
              <Form.Item name={['Binding', 'AddressAssignMode']} label={t('device.addressMode')} tooltip={t('device.addressModeHelp')}>
                <Radio.Group buttonStyle="solid">
                  <Radio.Button value="auto">{t('device.auto')}</Radio.Button>
                  <Radio.Button value="manual">{t('device.manual')}</Radio.Button>
                </Radio.Group>
              </Form.Item>
              {addressAssignMode === 'manual' ? (
                <>
                  <Form.Item name={['Binding', 'IPv4CIDR']} label={t('device.ipv4Cidr')}><Input placeholder={addressPlaceholders.ipv4} /></Form.Item>
                  <Form.Item name={['Binding', 'IPv6CIDR']} label={t('device.ipv6Cidr')}><Input placeholder={addressPlaceholders.ipv6} /></Form.Item>
                  <Form.Item name={['Binding', 'Gateway']} label={t('device.gateway')}><Input placeholder={addressPlaceholders.gateway} /></Form.Item>
                </>
              ) : null}
            </>
          ) : null}
        </>
      )}
    </>
  );
}
