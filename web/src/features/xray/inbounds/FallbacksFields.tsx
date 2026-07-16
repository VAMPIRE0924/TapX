import { Button, Card, Col, Empty, Form, Input, InputNumber, Row, Select, Space, Tooltip, type FormInstance } from 'antd';
import { ArrowDownOutlined, ArrowUpOutlined, DeleteOutlined, PlusOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import type { TapxEndpoint } from '../../../shared/api';
import { useI18n } from '../../../i18n/I18nProvider';

type JsonObject = Record<string, unknown>;

export function FallbacksFields({
  form,
  listeners,
  listenerID,
  enabled,
}: {
  form: FormInstance;
  listeners: TapxEndpoint[];
  listenerID?: string;
  enabled: boolean;
}) {
  const { t } = useI18n();
  const values = Form.useWatch(['settings', 'fallbacks'], form) as JsonObject[] | undefined;
  const candidates = listeners.filter((item) => item.ID !== listenerID && item.Enabled !== false && item.BindPort);
  const options = candidates.map((item) => ({
    value: item.ID,
    label: `${item.Name || item.ID} · ${item.Transport || 'xray'}:${item.BindPort}`,
  }));

  if (!enabled) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('listener.fallback.needsSecurity')} />;
  }

  function childIDFor(row: JsonObject | undefined): string | undefined {
    const dest = String(row?.dest || '');
    return candidates.find((item) => fallbackDestination(item) === dest)?.ID;
  }

  function applyChild(index: number, id?: string) {
    if (!id) return;
    const child = candidates.find((item) => item.ID === id);
    if (!child) return;
    const current = objectValue(form.getFieldValue(['settings', 'fallbacks', index]));
    const derived = deriveFallback(child);
    form.setFieldValue(['settings', 'fallbacks', index], {
      ...current,
      name: current.name || derived.name,
      alpn: current.alpn || derived.alpn,
      path: current.path || derived.path,
      dest: derived.dest,
      xver: current.xver || derived.xver,
    });
  }

  function addAll(add: (value: JsonObject) => void) {
    const destinations = new Set((values || []).map((item) => String(item?.dest || '')));
    for (const child of candidates) {
      const fallback = deriveFallback(child);
      if (!destinations.has(String(fallback.dest || ''))) add(fallback);
    }
  }

  return (
    <Form.List name={['settings', 'fallbacks']}>
      {(fields, { add, remove, move }) => (
        <Card
          size="small"
          title={(
            <Space size={4}>
              {t('listener.fallback.title')}
              <Tooltip title={t('listener.fallback.help')}><QuestionCircleOutlined /></Tooltip>
            </Space>
          )}
          extra={(
            <Space size={8} wrap>
              <Button type="primary" ghost size="small" icon={<PlusOutlined />} onClick={() => add(emptyFallback())}>{t('listener.fallback.add')}</Button>
              <Button size="small" disabled={candidates.length === 0 || fields.length >= candidates.length} onClick={() => addAll(add)}>{t('listener.fallback.addAll')}</Button>
            </Space>
          )}
        >
          {fields.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('listener.fallback.empty')} />
          ) : fields.map((field, index) => {
            const row = values?.[index];
            return (
              <Card key={field.key} type="inner" size="small" style={{ marginBottom: 8 }} styles={{ body: { padding: 12 } }}>
                <Space.Compact block style={{ marginBottom: 8 }}>
                  <Select
                    aria-label={t('listener.fallback.pick')}
                    value={childIDFor(row)}
                    options={options}
                    placeholder={t('listener.fallback.pick')}
                    allowClear
                    showSearch
                    style={{ width: '100%' }}
                    onChange={(value) => applyChild(index, typeof value === 'string' ? value : undefined)}
                  />
                  <Button aria-label={t('common.moveUp')} disabled={index === 0} icon={<ArrowUpOutlined />} onClick={() => move(index, index - 1)} />
                  <Button aria-label={t('common.moveDown')} disabled={index === fields.length - 1} icon={<ArrowDownOutlined />} onClick={() => move(index, index + 1)} />
                  <Button aria-label={t('common.delete')} danger icon={<DeleteOutlined />} onClick={() => remove(field.name)} />
                </Space.Compact>
                <Row gutter={[8, 8]}>
                  <Col xs={24} sm={12}><Form.Item name={[field.name, 'name']} noStyle><Input prefix="SNI" placeholder={t('listener.fallback.any')} /></Form.Item></Col>
                  <Col xs={24} sm={12}><Form.Item name={[field.name, 'alpn']} noStyle><Input prefix="ALPN" placeholder={t('listener.fallback.any')} /></Form.Item></Col>
                  <Col xs={24} sm={12}><Form.Item name={[field.name, 'path']} noStyle><Input prefix="Path" placeholder="/" /></Form.Item></Col>
                  <Col xs={24} sm={12}><Form.Item name={[field.name, 'dest']} noStyle><Input prefix="Dest" placeholder={t('listener.fallback.destination')} /></Form.Item></Col>
                  <Col xs={24} sm={12}><Form.Item name={[field.name, 'xver']} noStyle><InputNumber prefix="xver" min={0} max={2} style={{ width: '100%' }} /></Form.Item></Col>
                </Row>
              </Card>
            );
          })}
        </Card>
      )}
    </Form.List>
  );
}

function deriveFallback(listener: TapxEndpoint): JsonObject {
  const stream = objectValue((listener as TapxEndpoint & { streamSettings?: JsonObject }).streamSettings);
  const tls = objectValue(stream.tlsSettings);
  const network = String(stream.network || '');
  const transport = objectValue(
    network === 'ws' ? stream.wsSettings
      : network === 'grpc' ? stream.grpcSettings
        : network === 'httpupgrade' ? stream.httpupgradeSettings
          : network === 'xhttp' ? stream.xhttpSettings
            : undefined,
  );
  const alpn = Array.isArray(tls.alpn) ? tls.alpn.map(String).filter(Boolean).join(',') : '';
  const sockopt = objectValue(stream.sockopt);
  return {
    name: String(tls.serverName || ''),
    alpn,
    path: String(network === 'grpc' ? transport.serviceName || '' : transport.path || ''),
    dest: fallbackDestination(listener),
    xver: sockopt.acceptProxyProtocol === true ? 1 : 0,
  };
}

function fallbackDestination(listener: TapxEndpoint): string {
  const host = String(listener.BindHost || '').trim();
  const normalized = !host || host === '0.0.0.0' || host === '::' || host === '[::]' ? '127.0.0.1' : host;
  return `${normalized}:${listener.BindPort || 0}`;
}

function emptyFallback(): JsonObject {
  return { name: '', alpn: '', path: '', dest: '', xver: 0 };
}

function objectValue(value: unknown): JsonObject {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as JsonObject : {};
}
