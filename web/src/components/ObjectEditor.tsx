import { useEffect, useMemo, useState } from 'react';
import { Button, Drawer, Form, Input, InputNumber, Select, Space, Switch, Tabs, Typography, message } from 'antd';

import type { AnyRecord } from '@/api';
import { deepClone } from '@/api';
import type { FieldDef, KindDef } from '@/schema';
import { allFields, getPath, normalizeObject, setPath } from '@/schema';
import { useI18n } from '@/i18n';
import { JsonEditor } from './JsonEditor';

interface ObjectEditorProps {
  open: boolean;
  kind: KindDef;
  value: AnyRecord;
  onClose: () => void;
  onSave: (value: AnyRecord) => Promise<void>;
}

function formName(path: string) {
  return path.split('.');
}

function toFormInitial(value: AnyRecord, fields: FieldDef[]) {
  const out = deepClone(value || {});
  for (const field of fields) {
    const current = getPath(value, field.path);
    if (field.type === 'list') {
      setPath(out, field.path, Array.isArray(current) ? current.join('\n') : '');
    }
    if (field.type === 'json') {
      setPath(out, field.path, current == null ? '' : JSON.stringify(current, null, 2));
    }
  }
  return out;
}

function collectFormValue(raw: AnyRecord, fields: FieldDef[]) {
  const next = deepClone(raw || {});
  for (const field of fields) {
    const current = getPath(next, field.path);
    if (field.type === 'number') {
      setPath(next, field.path, current === '' || current == null ? 0 : Number(current));
    }
    if (field.type === 'list') {
      setPath(next, field.path, String(current || '').split(/\r?\n/).map((line) => line.trim()).filter(Boolean));
    }
    if (field.type === 'json') {
      const text = String(current || '').trim();
      setPath(next, field.path, text ? JSON.parse(text) : null);
    }
  }
  return normalizeObject(next);
}

function renderInput(field: FieldDef) {
  switch (field.type) {
    case 'switch':
      return <Switch />;
    case 'number':
      return <InputNumber className="full-width" />;
    case 'select':
      return <Select options={(field.options || []).map((value) => ({ value, label: value || 'default' }))} />;
    case 'textarea':
    case 'list':
    case 'json':
      return <Input.TextArea rows={field.type === 'json' ? 8 : 4} spellCheck={false} />;
    default:
      return <Input spellCheck={false} placeholder={field.placeholder} />;
  }
}

export function ObjectEditor({ open, kind, value, onClose, onSave }: ObjectEditorProps) {
  const { t } = useI18n();
  const [form] = Form.useForm();
  const fields = useMemo(() => allFields(kind), [kind]);
  const [jsonText, setJsonText] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    form.setFieldsValue(toFormInitial(value, fields));
    setJsonText(JSON.stringify(value, null, 2));
  }, [fields, form, open, value]);

  function syncJsonFromForm() {
    try {
      setJsonText(JSON.stringify(collectFormValue(form.getFieldsValue(true), fields), null, 2));
    } catch {
      return;
    }
  }

  function applyJsonToForm() {
    try {
      const parsed = JSON.parse(jsonText);
      form.setFieldsValue(toFormInitial(parsed, fields));
      message.success('JSON applied');
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  async function submit() {
    setSaving(true);
    try {
      const parsed = JSON.parse(jsonText);
      await onSave(normalizeObject(parsed));
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  const items = kind.groups.map((group) => ({
    key: group.title,
    label: group.title,
    children: (
      <div className="object-field-grid">
        {group.fields.map((field) => (
          <Form.Item
            key={field.path}
            name={formName(field.path)}
            label={field.label}
            valuePropName={field.type === 'switch' ? 'checked' : 'value'}
            className={field.span === 2 ? 'span-2' : undefined}
          >
            {renderInput(field)}
          </Form.Item>
        ))}
      </div>
    ),
  }));

  items.push({
    key: 'advanced-json',
    label: t('advanced'),
    children: (
      <Space direction="vertical" className="full-width" size="middle">
        <Typography.Text type="secondary">Complete object JSON. Use this for parameters not exposed in the current group layout.</Typography.Text>
        <JsonEditor value={jsonText} onChange={setJsonText} minHeight="420px" maxHeight="720px" />
        <Button onClick={applyJsonToForm}>{t('import')}</Button>
      </Space>
    ),
  });

  return (
    <Drawer
      title={`${value?.ID ? t('edit') : t('add')} ${kind.title}`}
      width="min(980px, 96vw)"
      open={open}
      onClose={onClose}
      destroyOnClose
      extra={(
        <Space>
          <Button onClick={onClose}>{t('cancel')}</Button>
          <Button type="primary" loading={saving} onClick={submit}>{t('save')}</Button>
        </Space>
      )}
    >
      <Form form={form} layout="vertical" onValuesChange={syncJsonFromForm}>
        <Tabs items={items} />
      </Form>
    </Drawer>
  );
}
