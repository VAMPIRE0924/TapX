import { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Form, Input, InputNumber, Modal, Select, Space, Switch } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import type { TapxDevice, TapxEndpoint } from '../../shared/api';
import { labelDevice, labelEndpoint, splitList } from '../../shared/tapx-model';
import { buildBulkNames, parseBulkExpiry } from './userBulk';
import { useI18n } from '../../i18n/I18nProvider';
import { randomLowerAndNumber } from '../../shared/random';

type ListenerBindingModalProps = {
  mode: 'attach' | 'detach';
  open: boolean;
  count: number;
  listeners: TapxEndpoint[];
  saving: boolean;
  onClose: () => void;
  onSubmit: (listenerIds: string[]) => void | Promise<void>;
};

export function ListenerBindingModal({ mode, open, count, listeners, saving, onClose, onSubmit }: ListenerBindingModalProps) {
  const { t } = useI18n();
  const [listenerIds, setListenerIds] = useState<string[]>([]);
  useEffect(() => {
    if (open) setListenerIds([]);
  }, [open]);
  const options = useMemo(
    () => listeners.map((item) => ({ value: item.ID, label: labelEndpoint(item) })),
    [listeners],
  );
  const attaching = mode === 'attach';
  return (
    <Modal
      open={open}
      title={attaching ? t('user.bulk.attachTitle', { count }) : t('user.bulk.detachTitle', { count })}
      okText={attaching ? t('user.bulk.attach') : t('user.bulk.detach')}
      cancelText={t('user.cancel')}
      destroyOnHidden
      confirmLoading={saving}
      okButtonProps={{ disabled: listenerIds.length === 0, danger: !attaching }}
      onCancel={onClose}
      onOk={() => onSubmit(listenerIds)}
    >
      {options.length === 0 ? (
        <Alert type="info" showIcon title={t('user.bulk.noListeners')} />
      ) : (
        <>
          <Space style={{ marginBottom: 8 }}>
            <Button size="small" onClick={() => setListenerIds(options.map((item) => item.value))}>{t('user.selectAll')}</Button>
            <Button size="small" onClick={() => setListenerIds([])}>{t('user.clearAll')}</Button>
          </Space>
          <Form.Item label={t('user.listeners')} tooltip={attaching ? t('user.bulk.attachHelp') : t('user.bulk.detachHelp')}>
            <Select
              mode="multiple"
              showSearch
              autoFocus
              allowClear
              value={listenerIds}
              options={options}
              optionFilterProp="label"
              placeholder={t('user.selectListeners')}
              style={{ width: '100%' }}
              onChange={setListenerIds}
            />
          </Form.Item>
        </>
      )}
    </Modal>
  );
}

type BulkAdjustModalProps = {
  open: boolean;
  count: number;
  saving: boolean;
  onClose: () => void;
  onSubmit: (input: {
    addDays: number;
    addTrafficGB: number;
    uploadRateMbps?: number | null;
    downloadRateMbps?: number | null;
  }) => void | Promise<void>;
};

export function BulkAdjustModal({ open, count, saving, onClose, onSubmit }: BulkAdjustModalProps) {
  const { t } = useI18n();
  const [form] = Form.useForm<{
    addDays: number;
    addTrafficGB: number;
    uploadRateMbps?: number | null;
    downloadRateMbps?: number | null;
  }>();
  useEffect(() => {
    if (open) {
      form.resetFields();
      form.setFieldsValue({ addDays: 0, addTrafficGB: 0 });
    }
  }, [form, open]);
  return (
    <Modal
      open={open}
      title={t('user.bulk.adjustTitle', { count })}
      okText={t('user.bulk.apply')}
      cancelText={t('user.cancel')}
      destroyOnHidden
      confirmLoading={saving}
      onCancel={onClose}
      onOk={async () => onSubmit(await form.validateFields())}
    >
      <Form form={form} layout="vertical">
        <Form.Item name="addDays" label={t('user.bulk.addDays')} tooltip={t('user.bulk.adjustHelp')}>
          <InputNumber precision={0} step={1} placeholder="30" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="addTrafficGB" label={t('user.bulk.addTraffic')} tooltip={t('user.bulk.adjustHelp')}>
          <InputNumber step={1} placeholder="100" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="uploadRateMbps" label={t('user.bulk.uploadRate')} tooltip={t('user.bulk.rateLimitHelp')}>
          <InputNumber min={0} step={1} placeholder="100" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="downloadRateMbps" label={t('user.bulk.downloadRate')} tooltip={t('user.bulk.rateLimitHelp')}>
          <InputNumber min={0} step={1} placeholder="100" style={{ width: '100%' }} />
        </Form.Item>
      </Form>
    </Modal>
  );
}

export type BulkCreateInput = {
  emails: string[];
  listenerIds: string[];
  vKey: string;
  allowedDevices: string[];
  allowedIPs: string[];
  remark: string;
  trafficGB: number;
  uploadRateMbps: number;
  downloadRateMbps: number;
  expiresAt: number;
  trafficReset: string;
};

type BulkCreateDraft = {
  method: number;
  first: number;
  last: number;
  prefix: string;
  postfix: string;
  quantity: number;
  listenerIds: string[];
  vKey: string;
  allowedDevices: string[];
  allowedIPsText: string;
  remark: string;
  trafficGB: number;
  uploadRateMbps: number;
  downloadRateMbps: number;
  delayedStart: boolean;
  expiryText: string;
  expireDays: number;
  trafficReset: string;
};

type BulkCreateModalProps = {
  open: boolean;
  listeners: TapxEndpoint[];
  devices: TapxDevice[];
  saving: boolean;
  onClose: () => void;
  onSubmit: (input: BulkCreateInput) => void | Promise<void>;
};

export function BulkCreateModal({ open, listeners, devices, saving, onClose, onSubmit }: BulkCreateModalProps) {
  const { t } = useI18n();
  const [form] = Form.useForm<BulkCreateDraft>();
  const method = Form.useWatch('method', form) ?? 0;
  const delayedStart = Form.useWatch('delayedStart', form) === true;
  useEffect(() => {
    if (!open) return;
    form.setFieldsValue({
      method: 0, first: 1, last: 1, prefix: '', postfix: '', quantity: 1,
      listenerIds: [], vKey: '', allowedDevices: [], allowedIPsText: '', remark: '',
      trafficGB: 0, uploadRateMbps: 0, downloadRateMbps: 0,
      delayedStart: false, expiryText: '', expireDays: 0,
      trafficReset: 'never',
    });
  }, [form, open]);
  const listenerOptions = listeners.map((item) => ({ value: item.ID, label: labelEndpoint(item) }));
  const deviceOptions = devices.map((item) => ({ value: item.ID, label: labelDevice(item) }));

  async function submit() {
    const values = await form.validateFields();
    const emails = buildBulkNames(values, () => randomLowerAndNumber(10));
    const expiresAt = values.delayedStart
      ? -Math.max(0, Math.trunc(values.expireDays || 0)) * 86400
      : parseBulkExpiry(values.expiryText);
    if (expiresAt == null) {
      form.setFields([{ name: 'expiryText', errors: [t('user.bulk.validDateRequired')] }]);
      return;
    }
    await onSubmit({
      emails,
      listenerIds: values.listenerIds || [],
      vKey: values.vKey || '',
      allowedDevices: values.allowedDevices || [],
      allowedIPs: splitList(values.allowedIPsText || ''),
      remark: values.remark || '',
      trafficGB: values.trafficGB || 0,
      uploadRateMbps: values.uploadRateMbps || 0,
      downloadRateMbps: values.downloadRateMbps || 0,
      expiresAt,
      trafficReset: values.trafficReset || 'never',
    });
  }

  return (
    <Modal
      open={open}
      title={t('user.bulk.createTitle')}
      okText={t('common.create')}
      cancelText={t('common.close')}
      width={640}
      destroyOnHidden
      confirmLoading={saving}
      onCancel={onClose}
      onOk={submit}
      styles={{ body: { maxHeight: 'calc(100vh - 160px)', overflowY: 'auto' } }}
    >
      <Form form={form} colon={false} labelCol={{ sm: { span: 8 } }} wrapperCol={{ sm: { span: 14 } }}>
        <Form.Item name="listenerIds" label={t('user.listeners')} rules={[{ required: true, message: t('user.bulk.listenerRequired') }]}>
          <Select mode="multiple" showSearch optionFilterProp="label" options={listenerOptions} placeholder={t('user.selectListeners')} />
        </Form.Item>
        <Form.Item name="method" label={t('user.bulk.generationMethod')}>
          <Select options={[
            { value: 0, label: t('user.bulk.random') },
            { value: 1, label: t('user.bulk.randomPrefix') },
            { value: 2, label: t('user.bulk.randomPrefixIndex') },
            { value: 3, label: t('user.bulk.randomPrefixIndexSuffix') },
            { value: 4, label: t('user.bulk.prefixIndexSuffix') },
          ]} />
        </Form.Item>
        {method > 1 ? (
          <>
            <Form.Item name="first" label={t('user.bulk.firstIndex')}><InputNumber min={1} precision={0} /></Form.Item>
            <Form.Item name="last" label={t('user.bulk.lastIndex')}><InputNumber min={1} precision={0} /></Form.Item>
          </>
        ) : null}
        {method > 0 ? <Form.Item name="prefix" label={t('user.bulk.prefix')}><Input /></Form.Item> : null}
        {method > 2 ? <Form.Item name="postfix" label={t('user.bulk.suffix')}><Input /></Form.Item> : null}
        {method < 2 ? <Form.Item name="quantity" label={t('user.bulk.quantity')}><InputNumber min={1} max={1000} precision={0} /></Form.Item> : null}
        <Form.Item label="vKey" htmlFor="bulk-vkey">
          <Space.Compact style={{ display: 'flex' }}>
            <Form.Item name="vKey" noStyle>
              <Input id="bulk-vkey" style={{ flex: 1 }} placeholder={t('user.bulk.vkeyPlaceholder')} />
            </Form.Item>
            <Button icon={<ReloadOutlined />} aria-label={t('user.regenerateVkey')} onClick={() => form.setFieldValue('vKey', randomLowerAndNumber(24))} />
          </Space.Compact>
        </Form.Item>
        <Form.Item name="allowedIPsText" label={t('user.allowedSourceIp')} tooltip={t('user.allowedSourceIpHelp')}>
          <Input placeholder="10.8.0.10, 10.8.0.0/24" allowClear />
        </Form.Item>
        <Form.Item name="allowedDevices" label={t('user.allowedTunTap')}>
          <Select mode="multiple" options={deviceOptions} placeholder={t('user.noDeviceLimit')} maxTagCount="responsive" allowClear />
        </Form.Item>
        <Form.Item name="remark" label={t('user.remark')}><Input /></Form.Item>
        <Form.Item name="trafficGB" label={t('user.totalTraffic')}><InputNumber min={0} step={1} /></Form.Item>
        <Form.Item name="uploadRateMbps" label={t('user.bulk.uploadRate')} tooltip={t('user.rateLimitHelp')}>
          <InputNumber min={0} step={1} placeholder="100" />
        </Form.Item>
        <Form.Item name="downloadRateMbps" label={t('user.bulk.downloadRate')} tooltip={t('user.rateLimitHelp')}>
          <InputNumber min={0} step={1} placeholder="100" />
        </Form.Item>
        <Form.Item name="delayedStart" label={t('user.delayedStart')} valuePropName="checked"><Switch /></Form.Item>
        {delayedStart
          ? <Form.Item name="expireDays" label={t('user.validDays')}><InputNumber min={0} precision={0} /></Form.Item>
          : <Form.Item name="expiryText" label={t('user.expiry')}><Input placeholder={t('user.noExpiry')} /></Form.Item>}
        <Form.Item name="trafficReset" label={t('user.trafficReset')}>
          <Select options={[
            { value: 'never', label: t('user.resetNever') }, { value: 'hourly', label: t('user.resetHourly') },
            { value: 'daily', label: t('user.resetDaily') }, { value: 'weekly', label: t('user.resetWeekly') }, { value: 'monthly', label: t('user.resetMonthly') },
          ]} />
        </Form.Item>
      </Form>
    </Modal>
  );
}
