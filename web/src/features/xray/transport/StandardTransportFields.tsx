import { Form, Input, InputNumber, Switch } from 'antd';
import { HeaderMapEditor } from '../../../components/HeaderMapEditor';
import type { XrayDirection } from '../XrayFormFields';
import { useI18n } from '../../../i18n/I18nProvider';

export function KcpTransportFields({ direction }: { direction: XrayDirection }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['streamSettings', 'kcpSettings', 'mtu']} label="MTU" tooltip={t('xray.kcpMtuHelp')}>
        <InputNumber min={direction === 'inbound' ? 576 : 0} max={direction === 'inbound' ? 1460 : undefined} placeholder="1350" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'kcpSettings', 'tti']} label="TTI (ms)" tooltip={t('xray.kcpTtiHelp')}>
        <InputNumber min={direction === 'inbound' ? 10 : 0} max={direction === 'inbound' ? 100 : undefined} placeholder="50" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'kcpSettings', 'uplinkCapacity']} label={t('xray.uplinkMbps')} tooltip={t('xray.kcpCapacityHelp')}>
        <InputNumber min={0} placeholder="100" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'kcpSettings', 'downlinkCapacity']} label={t('xray.downlinkMbps')} tooltip={t('xray.kcpCapacityHelp')}>
        <InputNumber min={0} placeholder="100" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'kcpSettings', 'cwndMultiplier']} label={t('xray.cwndMultiplier')} tooltip={t('xray.kcpCwndHelp')}>
        <InputNumber min={1} placeholder="1" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'kcpSettings', 'maxSendingWindow']} label={t('xray.maxSendingWindow')} tooltip={t('xray.kcpWindowHelp')}>
        <InputNumber min={0} placeholder="1024" />
      </Form.Item>
    </>
  );
}

export function WsTransportFields({ direction }: { direction: XrayDirection }) {
  const { t } = useI18n();
  return (
    <>
      {direction === 'inbound' ? (
        <Form.Item name={['streamSettings', 'wsSettings', 'acceptProxyProtocol']} label="Proxy Protocol" tooltip={t('xray.proxyProtocolHelp')} valuePropName="checked">
          <Switch />
        </Form.Item>
      ) : null}
      <Form.Item name={['streamSettings', 'wsSettings', 'host']} label={t('xray.host')} tooltip={t('xray.hostHelp')}>
        <Input placeholder="edge.example.com" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'wsSettings', 'path']} label={t('xray.path')} tooltip={t('xray.pathHelp')}>
        <Input placeholder="/tapx" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'wsSettings', 'heartbeatPeriod']} label={t('xray.heartbeatPeriod')} tooltip={t('xray.heartbeatHelp')}>
        <InputNumber min={0} placeholder="30" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'wsSettings', 'headers']} label={t('xray.requestHeaders')} tooltip={t('xray.headersHelp')}>
        <HeaderMapEditor mode="v1" />
      </Form.Item>
    </>
  );
}

export function GrpcTransportFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['streamSettings', 'grpcSettings', 'serviceName']} label={t('xray.serviceName')} tooltip={t('xray.serviceNameHelp')}>
        <Input placeholder="tapx.grpc" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'grpcSettings', 'authority']} label="Authority" tooltip={t('xray.authorityHelp')}>
        <Input placeholder="edge.example.com" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'grpcSettings', 'multiMode']} label={t('xray.multiMode')} tooltip={t('xray.multiModeHelp')} valuePropName="checked">
        <Switch />
      </Form.Item>
    </>
  );
}

export function HttpUpgradeTransportFields({ direction }: { direction: XrayDirection }) {
  const { t } = useI18n();
  return (
    <>
      {direction === 'inbound' ? (
        <Form.Item name={['streamSettings', 'httpupgradeSettings', 'acceptProxyProtocol']} label="Proxy Protocol" tooltip={t('xray.proxyProtocolHelp')} valuePropName="checked">
          <Switch />
        </Form.Item>
      ) : null}
      <Form.Item name={['streamSettings', 'httpupgradeSettings', 'host']} label={t('xray.host')} tooltip={t('xray.hostHelp')}>
        <Input placeholder="edge.example.com" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'httpupgradeSettings', 'path']} label={t('xray.path')} tooltip={t('xray.pathHelp')}>
        <Input placeholder="/tapx" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'httpupgradeSettings', 'headers']} label={t('xray.requestHeaders')} tooltip={t('xray.headersHelp')}>
        <HeaderMapEditor mode="v1" />
      </Form.Item>
    </>
  );
}
