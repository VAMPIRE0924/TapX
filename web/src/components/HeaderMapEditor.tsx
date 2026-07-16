import { useEffect, useRef, useState } from 'react';
import { Button, Input, Space } from 'antd';
import { MinusOutlined, PlusOutlined } from '@ant-design/icons';
import { useI18n } from '../i18n/I18nProvider';

export type HeaderMapMode = 'v1' | 'v2';

export type HeaderMapValue =
  | Record<string, string>
  | Record<string, string[]>
  | undefined;

interface HeaderRow {
  name: string;
  value: string;
}

interface HeaderMapEditorProps {
  mode: HeaderMapMode;
  value?: HeaderMapValue;
  onChange?: (next: Record<string, string> | Record<string, string[]>) => void;
}

function mapToRows(value: HeaderMapValue): HeaderRow[] {
  if (!value || typeof value !== 'object') return [];
  const rows: HeaderRow[] = [];
  for (const [name, raw] of Object.entries(value)) {
    if (Array.isArray(raw)) {
      for (const item of raw) rows.push({ name, value: typeof item === 'string' ? item : String(item) });
    } else if (typeof raw === 'string') {
      rows.push({ name, value: raw });
    }
  }
  return rows;
}

function rowsToMap(rows: HeaderRow[], mode: HeaderMapMode): Record<string, string> | Record<string, string[]> {
  if (mode === 'v1') {
    const map: Record<string, string> = {};
    for (const row of rows) {
      if (row.name.trim()) map[row.name.trim()] = row.value || '';
    }
    return map;
  }
  const map: Record<string, string[]> = {};
  for (const row of rows) {
    const name = row.name.trim();
    if (!name) continue;
    map[name] = [...(map[name] || []), row.value || ''];
  }
  return map;
}

export function HeaderMapEditor({ mode, value, onChange }: HeaderMapEditorProps) {
  const { t } = useI18n();
  const [rows, setRows] = useState<HeaderRow[]>(() => mapToRows(value));
  const lastEmittedRef = useRef<string>(JSON.stringify(rowsToMap(rows, mode)));

  useEffect(() => {
    const incoming = JSON.stringify(value ?? {});
    if (incoming === lastEmittedRef.current) return;
    setRows(mapToRows(value));
    lastEmittedRef.current = incoming;
  }, [value]);

  function commit(next: HeaderRow[]) {
    setRows(next);
    const map = rowsToMap(next, mode);
    lastEmittedRef.current = JSON.stringify(map);
    onChange?.(map);
  }

  function updateRow(index: number, patch: Partial<HeaderRow>) {
    const next = rows.slice();
    next[index] = { ...next[index], ...patch };
    commit(next);
  }

  return (
    <div className="header-map-editor">
      {rows.map((row, index) => (
        <Space.Compact key={index} block style={{ marginBottom: 8 }}>
          <Input addonBefore={String(index + 1)} value={row.name} placeholder="Name" onChange={(event) => updateRow(index, { name: event.target.value })} />
          <Input value={row.value} placeholder="Value" onChange={(event) => updateRow(index, { value: event.target.value })} />
          <Button aria-label={t('common.delete')} icon={<MinusOutlined />} onClick={() => commit(rows.filter((_, i) => i !== index))} />
        </Space.Compact>
      ))}
      <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => commit([...rows, { name: '', value: '' }])}>
        {t('xray.add')}
      </Button>
    </div>
  );
}
