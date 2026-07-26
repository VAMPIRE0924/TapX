import { useEffect, useRef, useState } from 'react';
import { Form, Input, type FormInstance } from 'antd';

type ObjectValue = Record<string, unknown>;

export function AdvancedConfigEditor({ form, rows = 16 }: { form: FormInstance; rows?: number }) {
  const watched = Form.useWatch([], { form, preserve: true });
  const lastWritten = useRef('');
  const serialize = (value: unknown) => JSON.stringify(isObject(value) ? value : {}, null, 2);
  const [text, setText] = useState(() => serialize(form.getFieldsValue(true)));

  useEffect(() => {
    const next = serialize(form.getFieldsValue(true));
    if (next === lastWritten.current) return;
    lastWritten.current = next;
    setText(next);
  }, [form, watched]);

  return (
    <Input.TextArea
      value={text}
      rows={rows}
      spellCheck={false}
      onChange={(event) => {
        const next = event.target.value;
        setText(next);
        try {
          const parsed = JSON.parse(next) as unknown;
          if (!isObject(parsed)) return;
          form.setFieldsValue(parsed);
          lastWritten.current = JSON.stringify(parsed, null, 2);
        } catch {
          // Preserve incomplete JSON until it becomes valid.
        }
      }}
    />
  );
}

function isObject(value: unknown): value is ObjectValue {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}
