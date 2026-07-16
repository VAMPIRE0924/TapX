import { AutoComplete, Form, Input, InputNumber, Select, Switch, type FormInstance } from 'antd';
import { HeaderMapEditor } from '../../../components/HeaderMapEditor';
import { validateSessionIDLength, validateSessionIDTable } from '../../../shared/xhttp-session-id';
import type { XrayDirection } from '../XrayFormFields';
import { useI18n } from '../../../i18n/I18nProvider';

const sessionTables = [
  'ALPHABET',
  'Alphabet',
  'BASE36',
  'Base62',
  'HEX',
  'alphabet',
  'base36',
  'hex',
  'number',
].map((value) => ({ value, label: value }));

export function XhttpTransportFields({ form, direction }: { form: FormInstance; direction: XrayDirection }) {
  const { t } = useI18n();
  const mode = Form.useWatch(['streamSettings', 'xhttpSettings', 'mode'], form) as string | undefined;
  const paddingObfs = Form.useWatch(['streamSettings', 'xhttpSettings', 'xPaddingObfsMode'], form) === true;
  const sessionPlacement = Form.useWatch(['streamSettings', 'xhttpSettings', 'sessionIDPlacement'], form) as string | undefined;
  const sessionTable = Form.useWatch(['streamSettings', 'xhttpSettings', 'sessionIDTable'], form) as string | undefined;
  const seqPlacement = Form.useWatch(['streamSettings', 'xhttpSettings', 'seqPlacement'], form) as string | undefined;
  const uplinkPlacement = Form.useWatch(['streamSettings', 'xhttpSettings', 'uplinkDataPlacement'], form) as string | undefined;
  const xmuxEnabled = Form.useWatch(['streamSettings', 'xhttpSettings', 'enableXmux'], form) === true;

  return (
    <>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'host']} label={t('xray.host')}>
        <Input />
      </Form.Item>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'path']} label={t('xray.path')}>
        <Input />
      </Form.Item>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'mode']} label={t('xray.mode')}>
        <Select options={modeOptions} />
      </Form.Item>

      {direction === 'inbound' ? <InboundModeFields mode={mode} /> : null}
      {direction === 'inbound' ? (
        <Form.Item name={['streamSettings', 'xhttpSettings', 'serverMaxHeaderBytes']} label={t('xray.serverMaxHeaderBytes')}>
          <InputNumber min={0} placeholder="0" />
        </Form.Item>
      ) : null}
      <Form.Item name={['streamSettings', 'xhttpSettings', 'xPaddingBytes']} label={t('xray.paddingBytes')}>
        <Input />
      </Form.Item>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'headers']} label={t('xray.requestHeaders')}>
        <HeaderMapEditor mode="v1" />
      </Form.Item>
      {direction === 'inbound' ? (
        <>
          <UplinkMethodField mode={mode} />
          <PaddingObfsFields enabled={paddingObfs} />
          <PlacementFields
            direction={direction}
            mode={mode}
            sessionPlacement={sessionPlacement}
            sessionTable={sessionTable}
            seqPlacement={seqPlacement}
            uplinkPlacement={uplinkPlacement}
          />
          <Form.Item name={['streamSettings', 'xhttpSettings', 'noSSEHeader']} label={t('xray.noSseHeader')} valuePropName="checked">
            <Switch />
          </Form.Item>
        </>
      ) : (
        <>
          <PaddingObfsFields enabled={paddingObfs} />
          <UplinkMethodField mode={mode} />
          <PlacementFields
            direction={direction}
            mode={mode}
            sessionPlacement={sessionPlacement}
            sessionTable={sessionTable}
            seqPlacement={seqPlacement}
            uplinkPlacement={uplinkPlacement}
          />
          <OutboundModeFields mode={mode} />
          {mode === 'stream-up' || mode === 'stream-one' ? (
            <Form.Item name={['streamSettings', 'xhttpSettings', 'noGRPCHeader']} label={t('xray.noGrpcHeader')} valuePropName="checked">
              <Switch />
            </Form.Item>
          ) : null}
        </>
      )}
      <XmuxFields form={form} enabled={xmuxEnabled} />
    </>
  );
}

function InboundModeFields({ mode }: { mode?: string }) {
  const { t } = useI18n();
  return (
    <>
      {mode === 'packet-up' || mode === 'auto' ? (
        <>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'scMaxEachPostBytes']} label={t('xray.maxUploadSize')}>
            <Input />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'scMaxBufferedPosts']} label={t('xray.maxBufferedUploads')}>
            <InputNumber />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'scMinPostsIntervalMs']} label={t('xray.minUploadInterval')}>
            <Input placeholder="50-150" />
          </Form.Item>
        </>
      ) : null}
      {mode === 'stream-up' ? (
        <>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'scMaxBufferedPosts']} label={t('xray.maxBufferedUploads')}>
            <InputNumber />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'scStreamUpServerSecs']} label="Stream Up Server">
            <Input />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}

function OutboundModeFields({ mode }: { mode?: string }) {
  const { t } = useI18n();
  if (mode !== 'packet-up' && mode !== 'auto') return null;
  return (
    <>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'scMinPostsIntervalMs']} label={t('xray.minUploadInterval')}>
        <Input placeholder="50-150" />
      </Form.Item>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'scMaxEachPostBytes']} label={t('xray.maxUploadSize')}>
        <Input placeholder="1000000" />
      </Form.Item>
    </>
  );
}

function UplinkMethodField({ mode }: { mode?: string }) {
  const { t } = useI18n();
  return (
    <Form.Item name={['streamSettings', 'xhttpSettings', 'uplinkHTTPMethod']} label={t('xray.uplinkHttpMethod')}>
      <Select
        placeholder="POST"
        options={[
          { value: '', label: 'Default (POST)' },
          { value: 'POST', label: 'POST' },
          { value: 'PUT', label: 'PUT' },
          { value: 'GET', label: 'GET (packet-up only)', disabled: mode !== 'packet-up' },
        ]}
      />
    </Form.Item>
  );
}

function PaddingObfsFields({ enabled }: { enabled: boolean }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'xPaddingObfsMode']} label={t('xray.paddingObfsMode')} valuePropName="checked">
        <Switch />
      </Form.Item>
      {enabled ? (
        <>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xPaddingKey']} label="Padding Key">
            <Input placeholder="x_padding" />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xPaddingHeader']} label="Padding Header">
            <Input placeholder="X-Padding" />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xPaddingPlacement']} label="Padding Placement">
            <Select options={paddingPlacementOptions} />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xPaddingMethod']} label="Padding Method">
            <Select options={paddingMethodOptions} />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}

function PlacementFields({
  direction,
  mode,
  sessionPlacement,
  sessionTable,
  seqPlacement,
  uplinkPlacement,
}: {
  direction: XrayDirection;
  mode?: string;
  sessionPlacement?: string;
  sessionTable?: string;
  seqPlacement?: string;
  uplinkPlacement?: string;
}) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'sessionIDPlacement']} label={t('xray.sessionPlacement')}>
        <Select placeholder="path" options={placementOptions('path')} />
      </Form.Item>
      {sessionPlacement && sessionPlacement !== 'path' ? (
        <Form.Item name={['streamSettings', 'xhttpSettings', 'sessionIDKey']} label="Session Key">
          <Input placeholder="x_session" />
        </Form.Item>
      ) : null}
      <Form.Item
        name={['streamSettings', 'xhttpSettings', 'sessionIDTable']}
          label={t('xray.sessionIdAlphabet')}
        tooltip={t('xray.sessionIdAlphabetHelp')}
        rules={[{ validator: (rule, value) => validateSessionIDTable(rule, value, t) }]}
      >
        <AutoComplete allowClear options={sessionTables} placeholder="Base62" />
      </Form.Item>
      {sessionTable ? (
        <Form.Item
          name={['streamSettings', 'xhttpSettings', 'sessionIDLength']}
          label="Session ID Length"
          tooltip={t('xray.sessionIdLengthHelp')}
          rules={[{ validator: (rule, value) => validateSessionIDLength(rule, value, t) }]}
        >
          <Input placeholder="8-16" />
        </Form.Item>
      ) : null}
      <Form.Item name={['streamSettings', 'xhttpSettings', 'seqPlacement']} label={t('xray.sequencePlacement')}>
        <Select placeholder="path" options={placementOptions('path')} />
      </Form.Item>
      {seqPlacement && seqPlacement !== 'path' ? (
        <Form.Item name={['streamSettings', 'xhttpSettings', 'seqKey']} label="Sequence Key">
          <Input placeholder="x_seq" />
        </Form.Item>
      ) : null}
      {mode === 'packet-up' || (direction === 'outbound' && mode === 'auto') ? (
        <>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'uplinkDataPlacement']} label={t('xray.uplinkDataPlacement')}>
            <Select placeholder="body" options={uplinkPlacementOptions} />
          </Form.Item>
          {uplinkPlacement && uplinkPlacement !== 'body' ? (
            <>
              <Form.Item name={['streamSettings', 'xhttpSettings', 'uplinkDataKey']} label={t('xray.uplinkDataKey')}>
                <Input placeholder="x_data" />
              </Form.Item>
              {direction === 'outbound' ? (
                <Form.Item name={['streamSettings', 'xhttpSettings', 'uplinkChunkSize']} label={t('xray.uplinkChunkSize')}>
                  <InputNumber min={0} placeholder="0 (unlimited)" style={{ width: '100%' }} />
                </Form.Item>
              ) : null}
            </>
          ) : null}
        </>
      ) : null}
    </>
  );
}

function XmuxFields({ form, enabled }: { form: FormInstance; enabled: boolean }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name={['streamSettings', 'xhttpSettings', 'enableXmux']} label="XMUX" valuePropName="checked">
        <Switch
          onChange={(checked) => {
            if (checked && !form.getFieldValue(['streamSettings', 'xhttpSettings', 'xmux'])) {
              form.setFieldValue(['streamSettings', 'xhttpSettings', 'xmux'], defaultXmux());
            }
          }}
        />
      </Form.Item>
      {enabled ? (
        <>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xmux', 'maxConcurrency']} label={t('xray.maxConcurrency')}>
            <Input placeholder="16-32" />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xmux', 'maxConnections']} label={t('xray.maxConnections')}>
            <Input placeholder="0" />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xmux', 'cMaxReuseTimes']} label={t('xray.maxReuseTimes')}>
            <Input />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xmux', 'hMaxRequestTimes']} label={t('xray.maxRequestTimes')}>
            <Input placeholder="600-900" />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xmux', 'hMaxReusableSecs']} label={t('xray.maxReusableSeconds')}>
            <Input placeholder="1800-3000" />
          </Form.Item>
          <Form.Item name={['streamSettings', 'xhttpSettings', 'xmux', 'hKeepAlivePeriod']} label={t('xray.keepAlivePeriod')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}

const modeOptions = [
  { value: 'auto', label: 'auto' },
  { value: 'packet-up', label: 'packet-up' },
  { value: 'stream-up', label: 'stream-up' },
  { value: 'stream-one', label: 'stream-one' },
];

function defaultXmux(): Record<string, unknown> {
  return {
    maxConcurrency: '16-32',
    maxConnections: 6,
    cMaxReuseTimes: 0,
    hMaxRequestTimes: '600-900',
    hMaxReusableSecs: '1800-3000',
    hKeepAlivePeriod: 0,
  };
}

const paddingPlacementOptions = [
  { value: '', label: 'Default (queryInHeader)' },
  { value: 'queryInHeader', label: 'queryInHeader' },
  { value: 'header', label: 'header' },
  { value: 'cookie', label: 'cookie' },
  { value: 'query', label: 'query' },
];

const paddingMethodOptions = [
  { value: '', label: 'Default (repeat-x)' },
  { value: 'repeat-x', label: 'repeat-x' },
  { value: 'tokenish', label: 'tokenish' },
];

function placementOptions(defaultValue: string) {
  return [
    { value: '', label: `Default (${defaultValue})` },
    { value: 'path', label: 'path' },
    { value: 'header', label: 'header' },
    { value: 'cookie', label: 'cookie' },
    { value: 'query', label: 'query' },
  ];
}

const uplinkPlacementOptions = [
  { value: '', label: 'Default (body)' },
  { value: 'body', label: 'body' },
  { value: 'header', label: 'header' },
  { value: 'cookie', label: 'cookie' },
  { value: 'query', label: 'query' },
];
