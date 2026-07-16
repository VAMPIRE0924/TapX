import { useEffect, useRef } from 'react';
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { AutoComplete, Button, Divider, Form, Input, InputNumber, Select, Space, Switch, type FormInstance } from 'antd';
import { getUtlsOptions } from '../options';
import { useI18n } from '../../../i18n/I18nProvider';
import { randomBase64, randomLowerAndNumber } from '../../../shared/random';

const tcpNetworks = new Set(['raw', 'tcp', 'httpupgrade', 'ws', 'grpc', 'xhttp']);
const geckoPacketMin = 1;
const geckoPacketMax = 2048;
const defaultGeckoPacketSize = { min: 512, max: 1200 };

type FieldPath = Array<string | number>;

export function FinalMaskFields({
  name,
  network = '',
  protocol = '',
  showAll = false,
}: {
  name: FieldPath;
  network?: string;
  protocol?: string;
  showAll?: boolean;
}) {
  const form = Form.useFormInstance();
  const migrated = useRef(false);
  const hysteria = protocol === 'hysteria';
  const wireguard = protocol === 'wireguard';
  const showTcp = showAll || (!wireguard && tcpNetworks.has(network));
  const showUdp = showAll || hysteria || wireguard || network === 'kcp';
  const showQuic = showAll || hysteria || network === 'xhttp';
  const quicParams = Form.useWatch([...name, 'quicParams'], { form, preserve: true });

  useEffect(() => {
    if (migrated.current) return;
    migrated.current = true;
    const masks = form.getFieldValue([...name, 'tcp']);
    if (!Array.isArray(masks)) return;
    let changed = false;
    const next = masks.map((mask) => {
      if (!mask || typeof mask !== 'object') return mask;
      const item = mask as Record<string, unknown>;
      if (item.type !== 'fragment' || !item.settings || typeof item.settings !== 'object') return mask;
      const settings = { ...(item.settings as Record<string, unknown>) };
      if (!Array.isArray(settings.lengths) && typeof settings.length === 'string' && settings.length.trim()) {
        settings.lengths = [settings.length];
        changed = true;
      }
      if ('length' in settings) {
        delete settings.length;
        changed = true;
      }
      if (!Array.isArray(settings.delays) && typeof settings.delay === 'string' && settings.delay.trim()) {
        settings.delays = [settings.delay];
        changed = true;
      }
      if ('delay' in settings) {
        delete settings.delay;
        changed = true;
      }
      return changed ? { ...item, settings } : mask;
    });
    if (changed) form.setFieldValue([...name, 'tcp'], next);
  }, [form, name]);

  if (!showTcp && !showUdp && !showQuic) return null;

  return (
    <>
      {showTcp ? <TcpMasks base={name} form={form} /> : null}
      {showUdp ? (
        <UdpMasks
          base={name}
          form={form}
          hysteria={hysteria}
          wireguard={wireguard}
          network={network}
        />
      ) : null}
      {showQuic ? (
        <>
          <Form.Item label="QUIC Params">
            <Switch
              aria-label="QUIC Params"
              checked={quicParams != null}
              onChange={(checked) => form.setFieldValue([...name, 'quicParams'], checked ? defaultQuicParams() : undefined)}
            />
          </Form.Item>
          {quicParams != null ? <QuicParams base={[...name, 'quicParams']} form={form} /> : null}
        </>
      ) : null}
    </>
  );
}

function TcpMasks({ base, form }: { base: FieldPath; form: FormInstance }) {
  const { t } = useI18n();
  return (
    <Form.List name={[...base, 'tcp']}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label="TCP Masks">
            <Button
              type="primary"
              size="small"
              icon={<PlusOutlined />}
              aria-label={t('xray.add')}
              onClick={() => add({ type: 'fragment', settings: defaultTcpMask('fragment') })}
            />
          </Form.Item>
          {fields.map((field, index) => (
            <TcpMask
              key={field.key}
              fieldName={field.name}
              index={index + 1}
              listPath={[...base, 'tcp']}
              form={form}
              remove={() => remove(field.name)}
            />
          ))}
        </>
      )}
    </Form.List>
  );
}

function TcpMask({
  fieldName,
  index,
  listPath,
  form,
  remove,
}: {
  fieldName: number;
  index: number;
  listPath: FieldPath;
  form: FormInstance;
  remove: () => void;
}) {
  const absolute = [...listPath, fieldName];
  return (
    <div>
      <ItemDivider label={`TCP Mask ${index}`} remove={remove} />
      <Form.Item label="Type" name={[fieldName, 'type']}>
        <Select
          onChange={(type) => form.setFieldValue([...absolute, 'settings'], defaultTcpMask(type))}
          options={[
            { value: 'fragment', label: 'Fragment' },
            { value: 'header-custom', label: 'Header Custom' },
            { value: 'sudoku', label: 'Sudoku' },
          ]}
        />
      </Form.Item>
      <Form.Item noStyle shouldUpdate={(before, after) => getDeep(before, [...absolute, 'type']) !== getDeep(after, [...absolute, 'type'])}>
        {({ getFieldValue }) => {
          const type = getFieldValue([...absolute, 'type']);
          if (type === 'fragment') {
            return (
              <>
                <Form.Item label="Packets" name={[fieldName, 'settings', 'packets']} rules={[{ validator: validateFragmentPackets }]}>
                  <AutoComplete
                    options={['tlshello', '1-3', '1-5'].map((value) => ({ value, label: value }))}
                    placeholder="tlshello / 1-3"
                  />
                </Form.Item>
                <RangeList
                  name={[fieldName, 'settings', 'lengths']}
                  label="Lengths"
                  placeholder="100-200"
                  minItems={1}
                  validator={validateFragmentLength}
                />
                <RangeList
                  name={[fieldName, 'settings', 'delays']}
                  label="Delays"
                  placeholder="10-20 / 0"
                  validator={validateFragmentDelay}
                />
                <Form.Item label="Max Split" name={[fieldName, 'settings', 'maxSplit']}><Input /></Form.Item>
              </>
            );
          }
          if (type === 'sudoku') {
            return (
              <>
                <Form.Item label="Password" name={[fieldName, 'settings', 'password']}><Input /></Form.Item>
                <Form.Item label="ASCII" name={[fieldName, 'settings', 'ascii']}><Input /></Form.Item>
                <Form.Item label="Custom Table" name={[fieldName, 'settings', 'customTable']}><Input /></Form.Item>
                <Form.Item label="Custom Tables" name={[fieldName, 'settings', 'customTables']}>
                  <Select mode="tags" tokenSeparators={[',']} />
                </Form.Item>
                <Form.Item label="Padding Min" name={[fieldName, 'settings', 'paddingMin']}><InputNumber min={0} /></Form.Item>
                <Form.Item label="Padding Max" name={[fieldName, 'settings', 'paddingMax']}><InputNumber min={0} /></Form.Item>
              </>
            );
          }
          if (type === 'header-custom') {
            return <TcpHeaderGroups fieldName={fieldName} absolute={[...absolute, 'settings']} form={form} />;
          }
          return null;
        }}
      </Form.Item>
    </div>
  );
}

function RangeList({
  name,
  label,
  placeholder,
  minItems = 0,
  validator,
}: {
  name: FieldPath;
  label: string;
  placeholder: string;
  minItems?: number;
  validator: (_rule: unknown, value: unknown) => Promise<void>;
}) {
  const { t } = useI18n();
  return (
    <Form.List name={name}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label={label}>
            <Button type="primary" size="small" icon={<PlusOutlined />} aria-label={t('xray.add')} onClick={() => add('')} />
          </Form.Item>
          {fields.map((field, index) => (
            <Form.Item key={field.key} label={`#${index + 1}`} name={field.name} rules={[{ validator }]}>
              <Input
                placeholder={placeholder}
                suffix={fields.length > minItems ? <DeleteAction label={t('xray.remove')} action={() => remove(field.name)} /> : null}
              />
            </Form.Item>
          ))}
        </>
      )}
    </Form.List>
  );
}

function TcpHeaderGroups({ fieldName, absolute, form }: { fieldName: number; absolute: FieldPath; form: FormInstance }) {
  const { t } = useI18n();
  return (
    <>
      {(['clients', 'servers'] as const).map((groupKey) => (
        <Form.List key={groupKey} name={[fieldName, 'settings', groupKey]}>
          {(groups, { add, remove }) => (
            <>
              <Form.Item label={groupKey === 'clients' ? 'Clients' : 'Servers'}>
                <Button
                  type="primary"
                  size="small"
                  icon={<PlusOutlined />}
                  aria-label={t('xray.add')}
                  onClick={() => add([defaultClientServerItem(true)])}
                />
              </Form.Item>
              {groups.map((group, groupIndex) => (
                <div key={group.key}>
                  <ItemDivider label={`${groupKey === 'clients' ? 'Clients' : 'Servers'} Group ${groupIndex + 1}`} remove={() => remove(group.name)} />
                  <Form.List name={[group.name]}>
                    {(items, itemActions) => (
                      <>
                        <Form.Item label="Items">
                          <Button
                            size="small"
                            icon={<PlusOutlined />}
                            aria-label={t('xray.add')}
                            onClick={() => itemActions.add(defaultClientServerItem(true))}
                          />
                        </Form.Item>
                        {items.map((item, itemIndex) => (
                          <PacketItem
                            key={item.key}
                            fieldName={item.name}
                            index={itemIndex + 1}
                            absolute={[...absolute, groupKey, group.name, item.name]}
                            form={form}
                            delayMode="number"
                            remove={() => itemActions.remove(item.name)}
                          />
                        ))}
                      </>
                    )}
                  </Form.List>
                </div>
              ))}
            </>
          )}
        </Form.List>
      ))}
    </>
  );
}

function UdpMasks({
  base,
  form,
  hysteria,
  wireguard,
  network,
}: {
  base: FieldPath;
  form: FormInstance;
  hysteria: boolean;
  wireguard: boolean;
  network: string;
}) {
  const { t } = useI18n();
  return (
    <Form.List name={[...base, 'udp']}>
      {(fields, { add, remove }) => (
        <>
          <Form.Item label="UDP Masks">
            <Button
              type="primary"
              size="small"
              icon={<PlusOutlined />}
              aria-label={t('xray.add')}
              onClick={() => {
                const type = hysteria || wireguard ? 'salamander' : 'mkcp-legacy';
                add({ type, settings: defaultUdpMask(type) });
              }}
            />
          </Form.Item>
          {fields.map((field, index) => (
            <UdpMask
              key={field.key}
              fieldName={field.name}
              index={index + 1}
              listPath={[...base, 'udp']}
              form={form}
              hysteria={hysteria}
              wireguard={wireguard}
              network={network}
              remove={() => remove(field.name)}
            />
          ))}
        </>
      )}
    </Form.List>
  );
}

function UdpMask({
  fieldName,
  index,
  listPath,
  form,
  hysteria,
  wireguard,
  network,
  remove,
}: {
  fieldName: number;
  index: number;
  listPath: FieldPath;
  form: FormInstance;
  hysteria: boolean;
  wireguard: boolean;
  network: string;
  remove: () => void;
}) {
  const absolute = [...listPath, fieldName];
  const options = hysteria
    ? [{ value: 'salamander', label: 'Salamander (Hysteria2)' }]
    : [
        ...(wireguard ? [{ value: 'salamander', label: 'Salamander' }] : []),
        { value: 'mkcp-legacy', label: 'mKCP Legacy' },
        { value: 'xdns', label: 'xDNS' },
        { value: 'xicmp', label: 'xICMP' },
        { value: 'realm', label: 'Realm' },
        { value: 'header-custom', label: 'Header Custom' },
        { value: 'noise', label: 'Noise' },
      ];

  const changeType = (type: string) => {
    form.setFieldValue([...absolute, 'settings'], defaultUdpMask(type));
    if (network === 'kcp') {
      form.setFieldValue([...listPath.slice(0, -1), 'kcpSettings', 'mtu'], type === 'xdns' ? 900 : 1350);
    }
  };

  return (
    <div>
      <ItemDivider label={`UDP Mask ${index}`} remove={remove} />
      <Form.Item label="Type" name={[fieldName, 'type']}><Select options={options} onChange={changeType} /></Form.Item>
      <Form.Item noStyle shouldUpdate={(before, after) => getDeep(before, [...absolute, 'type']) !== getDeep(after, [...absolute, 'type'])}>
        {({ getFieldValue }) => {
          const type = getFieldValue([...absolute, 'type']);
          if (type === 'salamander') return <Salamander fieldName={fieldName} absolute={absolute} form={form} />;
          if (type === 'mkcp-legacy') {
            return (
              <>
                <Form.Item label="Header" name={[fieldName, 'settings', 'header']}>
                  <Select options={[
                    { value: '', label: 'Original / AES-128-GCM' },
                    { value: 'dns', label: 'DNS' },
                    { value: 'dtls', label: 'DTLS 1.2' },
                    { value: 'srtp', label: 'SRTP' },
                    { value: 'utp', label: 'uTP' },
                    { value: 'wechat', label: 'WeChat Video' },
                    { value: 'wireguard', label: 'WireGuard' },
                  ]} />
                </Form.Item>
                <Form.Item label="Value" name={[fieldName, 'settings', 'value']}>
                  <Input placeholder="password (AES-128-GCM) or domain (DNS header)" />
                </Form.Item>
              </>
            );
          }
          if (type === 'xdns') {
            return <Form.Item label="Domains" name={[fieldName, 'settings', 'domains']}><Select mode="tags" tokenSeparators={[',']} /></Form.Item>;
          }
          if (type === 'xicmp') {
            return (
              <>
                <Form.Item label="Dgram" name={[fieldName, 'settings', 'dgram']} valuePropName="checked"><Switch /></Form.Item>
                <Form.Item label="IPs" name={[fieldName, 'settings', 'ips']}><Select mode="tags" tokenSeparators={[',']} /></Form.Item>
              </>
            );
          }
          if (type === 'realm') return <Realm fieldName={fieldName} />;
          if (type === 'header-custom') return <UdpHeader fieldName={fieldName} absolute={[...absolute, 'settings']} form={form} />;
          if (type === 'noise') return <Noise fieldName={fieldName} absolute={[...absolute, 'settings']} form={form} />;
          return null;
        }}
      </Form.Item>
    </div>
  );
}

function Salamander({ fieldName, absolute, form }: { fieldName: number; absolute: FieldPath; form: FormInstance }) {
  const { t } = useI18n();
  const path = [...absolute, 'settings', 'packetSize'];
  const packetSize = Form.useWatch(path, { form, preserve: true });
  const gecko = typeof packetSize === 'string' && packetSize.trim() !== '';
  return (
    <>
      <Form.Item
        label="Mode"
        tooltip={gecko
          ? 'Salamander plus Gecko: splits each packet into random-padded fragments sized within the range below, defeating packet-length fingerprinting. Stored as Salamander with packetSize.'
          : 'Scrambles each packet into random-looking bytes.'}
      >
        <Select
          value={gecko ? 'gecko' : 'salamander'}
          onChange={(mode) => form.setFieldValue(path, mode === 'gecko' ? normalizeGeckoPacketSize(packetSize) : undefined)}
          options={[
            { value: 'salamander', label: 'Salamander' },
            { value: 'gecko', label: 'Gecko experimental' },
          ]}
        />
      </Form.Item>
      <Form.Item label="Password">
        <Space.Compact block>
          <Form.Item name={[fieldName, 'settings', 'password']} noStyle>
            <Input placeholder="Obfuscation password" style={{ width: 'calc(100% - 32px)' }} />
          </Form.Item>
          <Button
            icon={<ReloadOutlined />}
            aria-label={t('xray.regenerate')}
            onClick={() => form.setFieldValue([...absolute, 'settings', 'password'], randomLowerAndNumber(16))}
          />
        </Space.Compact>
      </Form.Item>
      {gecko ? (
        <Form.Item
          label="Packet size"
          name={[fieldName, 'settings', 'packetSize']}
          rules={[{ validator: validateGeckoPacketSize }]}
          tooltip="Packet size range, for example 512-1200."
        >
          <GeckoPacketSize />
        </Form.Item>
      ) : null}
    </>
  );
}

function GeckoPacketSize({ value, onChange }: { value?: string; onChange?: (value: string) => void }) {
  const [minText = '', maxText = ''] = String(value || '').split('-', 2);
  const min = /^\d+$/.test(minText) ? Number(minText) : null;
  const max = /^\d+$/.test(maxText) ? Number(maxText) : null;
  return (
    <Space.Compact block>
      <InputNumber
        prefix="Min"
        min={geckoPacketMin}
        max={geckoPacketMax}
        precision={0}
        value={min}
        placeholder={String(defaultGeckoPacketSize.min)}
        onChange={(next) => onChange?.(`${next ?? ''}-${max ?? ''}`)}
        style={{ width: '50%' }}
      />
      <InputNumber
        prefix="Max"
        min={geckoPacketMin}
        max={geckoPacketMax}
        precision={0}
        value={max}
        placeholder={String(defaultGeckoPacketSize.max)}
        onChange={(next) => onChange?.(`${min ?? ''}-${next ?? ''}`)}
        style={{ width: '50%' }}
      />
    </Space.Compact>
  );
}

function Realm({ fieldName }: { fieldName: number }) {
  const { t } = useI18n();
  const utlsOptions = getUtlsOptions(t);
  return (
    <>
      <Form.Item label="URL" name={[fieldName, 'settings', 'url']}><Input placeholder="realm://token@host:port/id" /></Form.Item>
      <Form.Item label="STUN Servers" name={[fieldName, 'settings', 'stunServers']}>
        <Select mode="tags" tokenSeparators={[',']} placeholder="host:port" />
      </Form.Item>
      <Divider plain style={{ margin: '8px 0' }}>TLS (optional)</Divider>
      <Form.Item label="Server Name" name={[fieldName, 'settings', 'tlsConfig', 'serverName']} tooltip="TLS SNI. Leave empty to disable TLS for this Realm.">
        <Input placeholder="realm.example.com" />
      </Form.Item>
      <Form.Item label="ALPN" name={[fieldName, 'settings', 'tlsConfig', 'alpn']}>
        <Select mode="multiple" options={['h3', 'h2', 'http/1.1'].map((value) => ({ value, label: value }))} />
      </Form.Item>
      <Form.Item label="Fingerprint" name={[fieldName, 'settings', 'tlsConfig', 'fingerprint']}>
        <Select allowClear options={utlsOptions.filter((option) => option.value)} />
      </Form.Item>
      <Form.Item label="Allow Insecure" name={[fieldName, 'settings', 'tlsConfig', 'allowInsecure']} valuePropName="checked">
        <Switch />
      </Form.Item>
    </>
  );
}

function UdpHeader({ fieldName, absolute, form }: { fieldName: number; absolute: FieldPath; form: FormInstance }) {
  const { t } = useI18n();
  return (
    <>
      {(['client', 'server'] as const).map((key) => (
        <Form.List key={key} name={[fieldName, 'settings', key]}>
          {(items, { add, remove }) => (
            <>
              <Form.Item label={key === 'client' ? 'Client' : 'Server'}>
                <Button type="primary" size="small" icon={<PlusOutlined />} aria-label={t('xray.add')} onClick={() => add(defaultClientServerItem(false))} />
              </Form.Item>
              {items.map((item, index) => (
                <PacketItem
                  key={item.key}
                  fieldName={item.name}
                  index={index + 1}
                  absolute={[...absolute, key, item.name]}
                  form={form}
                  remove={() => remove(item.name)}
                />
              ))}
            </>
          )}
        </Form.List>
      ))}
    </>
  );
}

function Noise({ fieldName, absolute, form }: { fieldName: number; absolute: FieldPath; form: FormInstance }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label="Reset" name={[fieldName, 'settings', 'reset']}><InputNumber min={0} /></Form.Item>
      <Form.List name={[fieldName, 'settings', 'noise']}>
        {(items, { add, remove }) => (
          <>
            <Form.Item label="Noise">
              <Button type="primary" size="small" icon={<PlusOutlined />} aria-label={t('xray.add')} onClick={() => add(defaultNoiseItem())} />
            </Form.Item>
            {items.map((item, index) => (
              <PacketItem
                key={item.key}
                fieldName={item.name}
                index={index + 1}
                absolute={[...absolute, 'noise', item.name]}
                form={form}
                delayMode="string"
                remove={() => remove(item.name)}
              />
            ))}
          </>
        )}
      </Form.List>
    </>
  );
}

function PacketItem({
  fieldName,
  index,
  absolute,
  form,
  delayMode,
  remove,
}: {
  fieldName: number;
  index: number;
  absolute: FieldPath;
  form: FormInstance;
  delayMode?: 'number' | 'string';
  remove: () => void;
}) {
  const { t } = useI18n();
  const type = Form.useWatch([...absolute, 'type'], { form, preserve: true }) as string | undefined;
  const changeType = (next: string) => {
    if (next === 'base64') form.setFieldValue([...absolute, 'packet'], randomBase64());
    else if (next === 'array') {
      form.setFieldValue([...absolute, 'rand'], delayMode === 'string' ? '1-8192' : 0);
      form.setFieldValue([...absolute, 'packet'], []);
    } else form.setFieldValue([...absolute, 'packet'], '');
  };
  return (
    <div>
      <ItemDivider label={`Item ${index}`} remove={remove} />
      <Form.Item label="Type" name={[fieldName, 'type']}>
        <Select onChange={changeType} options={[
          { value: 'array', label: 'Array' },
          { value: 'str', label: 'String' },
          { value: 'hex', label: 'Hex' },
          { value: 'base64', label: 'Base64' },
        ]} />
      </Form.Item>
      {delayMode === 'number' ? <Form.Item label="Delay (ms)" name={[fieldName, 'delay']}><InputNumber min={0} /></Form.Item> : null}
      {delayMode === 'string' ? <Form.Item label="Delay" name={[fieldName, 'delay']}><Input placeholder="10-20" /></Form.Item> : null}
      {type === 'array' ? (
        <>
          <Form.Item label="Rand" name={[fieldName, 'rand']}>
            {delayMode === 'string' ? <Input placeholder="0 or 1-8192" /> : <InputNumber min={0} />}
          </Form.Item>
          <Form.Item
            label="Rand Range"
            name={[fieldName, 'randRange']}
            normalize={(value) => value === '' ? undefined : value}
            rules={[{ validator: validateRandRange }]}
          >
            <Input placeholder="0-255" />
          </Form.Item>
        </>
      ) : type === 'base64' ? (
        <Form.Item label="Packet">
          <Space.Compact block>
            <Form.Item name={[fieldName, 'packet']} noStyle><Input placeholder="binary data" /></Form.Item>
            <Button icon={<ReloadOutlined />} aria-label={t('xray.regenerate')} onClick={() => form.setFieldValue([...absolute, 'packet'], randomBase64())} />
          </Space.Compact>
        </Form.Item>
      ) : (
        <Form.Item label="Packet" name={[fieldName, 'packet']}><Input placeholder="binary data" /></Form.Item>
      )}
    </div>
  );
}

function QuicParams({ base, form }: { base: FieldPath; form: FormInstance }) {
  const congestion = Form.useWatch([...base, 'congestion'], { form, preserve: true });
  const udpHop = Form.useWatch([...base, 'udpHop'], { form, preserve: true });
  return (
    <>
      <Form.Item label="Congestion" name={[...base, 'congestion']}>
        <Select options={[
          { value: 'reno', label: 'Reno' },
          { value: 'bbr', label: 'BBR' },
          { value: 'brutal', label: 'Brutal' },
          { value: 'force-brutal', label: 'Force Brutal' },
        ]} />
      </Form.Item>
      {congestion === 'bbr' ? (
        <Form.Item label="BBR Profile" name={[...base, 'bbrProfile']}>
          <Select allowClear placeholder="standard" options={['conservative', 'standard', 'aggressive'].map((value) => ({ value, label: titleCase(value) }))} />
        </Form.Item>
      ) : null}
      <Form.Item label="Debug" name={[...base, 'debug']} valuePropName="checked"><Switch /></Form.Item>
      {congestion === 'brutal' || congestion === 'force-brutal' ? (
        <>
          <Form.Item label="Brutal Up" name={[...base, 'brutalUp']} tooltip="Configured upstream bandwidth."><Input placeholder="60 mbps" /></Form.Item>
          <Form.Item label="Brutal Down" name={[...base, 'brutalDown']} tooltip="Configured downstream bandwidth."><Input placeholder="100 mbps" /></Form.Item>
        </>
      ) : null}
      <Form.Item label="UDP Hop">
        <Switch
          aria-label="UDP Hop"
          checked={udpHop != null}
          onChange={(checked) => form.setFieldValue([...base, 'udpHop'], checked ? { ports: '20000-50000', interval: '5-10' } : undefined)}
        />
      </Form.Item>
      {udpHop != null ? (
        <>
          <Form.Item label="Hop Ports" name={[...base, 'udpHop', 'ports']} tooltip="UDP port or port range used for hopping."><Input placeholder="20000-50000" /></Form.Item>
          <Form.Item label="Hop Interval (s)" name={[...base, 'udpHop', 'interval']} tooltip="Interval or interval range between port changes."><Input placeholder="5-10" /></Form.Item>
        </>
      ) : null}
      <Form.Item label="Max Idle Timeout (s)" name={[...base, 'maxIdleTimeout']}><InputNumber min={4} max={120} /></Form.Item>
      <Form.Item label="Keep Alive Period (s)" name={[...base, 'keepAlivePeriod']}><InputNumber min={2} max={60} /></Form.Item>
      <Form.Item label="Disable Path MTU Dis" name={[...base, 'disablePathMTUDiscovery']} valuePropName="checked"><Switch /></Form.Item>
      <Form.Item label="Max Incoming Streams" name={[...base, 'maxIncomingStreams']}><InputNumber min={8} placeholder="1024" /></Form.Item>
      <Form.Item label="Init Stream Window" name={[...base, 'initStreamReceiveWindow']}><InputNumber min={16384} placeholder="8388608" /></Form.Item>
      <Form.Item label="Max Stream Window" name={[...base, 'maxStreamReceiveWindow']}><InputNumber min={16384} placeholder="8388608" /></Form.Item>
      <Form.Item label="Init Conn Window" name={[...base, 'initConnectionReceiveWindow']}><InputNumber min={16384} placeholder="20971520" /></Form.Item>
      <Form.Item label="Max Conn Window" name={[...base, 'maxConnectionReceiveWindow']}><InputNumber min={16384} placeholder="20971520" /></Form.Item>
    </>
  );
}

function ItemDivider({ label, remove }: { label: string; remove: () => void }) {
  const { t } = useI18n();
  return (
    <Divider style={{ margin: 0 }}>
      {label}
      <DeleteAction label={t('xray.remove')} action={remove} />
    </Divider>
  );
}

function DeleteAction({ label, action }: { label: string; action: () => void }) {
  return (
    <DeleteOutlined
      className="danger-icon"
      role="button"
      tabIndex={0}
      aria-label={label}
      onClick={action}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          action();
        }
      }}
    />
  );
}

function defaultTcpMask(type: string): Record<string, unknown> {
  if (type === 'fragment') return { packets: '1-3', lengths: ['100-200'], delays: [], maxSplit: '' };
  if (type === 'sudoku') return { password: '', ascii: '', customTable: '', customTables: [], paddingMin: 0, paddingMax: 0 };
  if (type === 'header-custom') return { clients: [], servers: [] };
  return {};
}

function defaultUdpMask(type: string): Record<string, unknown> {
  if (type === 'salamander') return { password: '' };
  if (type === 'mkcp-legacy') return { header: '', value: '' };
  if (type === 'xdns') return { domains: [] };
  if (type === 'xicmp') return { dgram: false, ips: [] };
  if (type === 'realm') return { url: '', stunServers: [] };
  if (type === 'header-custom') return { client: [], server: [] };
  if (type === 'noise') return { reset: 0, noise: [] };
  return {};
}

function defaultClientServerItem(withDelay: boolean): Record<string, unknown> {
  return { ...(withDelay ? { delay: 0 } : {}), rand: 0, randRange: '0-255', type: 'array', packet: [] };
}

function defaultNoiseItem(): Record<string, unknown> {
  return { rand: '1-8192', randRange: '0-255', type: 'array', packet: [], delay: '10-20' };
}

function defaultQuicParams(): Record<string, unknown> {
  return {
    congestion: 'bbr',
    debug: false,
    maxIdleTimeout: 30,
    keepAlivePeriod: 10,
    disablePathMTUDiscovery: false,
    maxIncomingStreams: 1024,
    initStreamReceiveWindow: 8388608,
    maxStreamReceiveWindow: 8388608,
    initConnectionReceiveWindow: 20971520,
    maxConnectionReceiveWindow: 20971520,
  };
}

function getDeep(value: unknown, path: FieldPath): unknown {
  let current = value;
  for (const key of path) {
    if (!current || typeof current !== 'object') return undefined;
    current = (current as Record<string | number, unknown>)[key];
  }
  return current;
}

function validateFragmentPackets(_rule: unknown, value: unknown): Promise<void> {
  const text = String(value ?? '').trim();
  return !text || text === 'tlshello' || /^\d+-\d+$/.test(text)
    ? Promise.resolve()
    : Promise.reject(new Error('Use "tlshello" or a packet range like 1-3'));
}

function validateFragmentLength(_rule: unknown, value: unknown): Promise<void> {
  const text = String(value ?? '').trim();
  const minimum = Number(text.split('-')[0]);
  return text && Number.isFinite(minimum) && minimum > 0
    ? Promise.resolve()
    : Promise.reject(new Error('Length minimum must be greater than 0 (e.g. 100-200)'));
}

function validateFragmentDelay(_rule: unknown, value: unknown): Promise<void> {
  const text = String(value ?? '').trim();
  return /^\d+(?:-\d+)?$/.test(text)
    ? Promise.resolve()
    : Promise.reject(new Error("Delay is required; remove the row if you don't want a delay"));
}

function validateRandRange(_rule: unknown, value: unknown): Promise<void> {
  const text = String(value ?? '').trim();
  if (!text) return Promise.resolve();
  const match = /^(\d{1,3})(?:-(\d{1,3}))?$/.exec(text);
  if (!match) return Promise.reject(new Error('Use a byte value or range like 0-255'));
  const start = Number(match[1]);
  const end = match[2] === undefined ? start : Number(match[2]);
  return start <= 255 && end <= 255
    ? Promise.resolve()
    : Promise.reject(new Error('randRange bytes must be within 0-255'));
}

function validateGeckoPacketSize(_rule: unknown, value: unknown): Promise<void> {
  const match = /^(\d+)-(\d+)$/.exec(String(value ?? '').trim());
  if (match) {
    const min = Number(match[1]);
    const max = Number(match[2]);
    if (Number.isSafeInteger(min) && Number.isSafeInteger(max) && min >= geckoPacketMin && max >= min && max <= geckoPacketMax) {
      return Promise.resolve();
    }
  }
  return Promise.reject(new Error(`Use a range like 512-1200 (${geckoPacketMin}-${geckoPacketMax}, max >= min)`));
}

function normalizeGeckoPacketSize(value: unknown): string {
  const match = /^(\d+)-(\d+)$/.exec(String(value ?? '').trim());
  if (match) {
    const min = Number(match[1]);
    const max = Number(match[2]);
    if (min >= geckoPacketMin && max >= min && max <= geckoPacketMax) return `${min}-${max}`;
  }
  return `${defaultGeckoPacketSize.min}-${defaultGeckoPacketSize.max}`;
}

function titleCase(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
