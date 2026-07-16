import { useEffect, useRef, useState } from 'react';
import { Form, Input, type FormInstance } from 'antd';
import type { NamePath } from 'antd/es/form/interface';

type ListenerFormShape = Record<string, unknown>;

export function AdvancedInboundSliceEditor({
  form,
  path,
  wrapKey,
  rows = 12,
}: {
  form: FormInstance;
  path: NamePath;
  wrapKey: string;
  rows?: number;
}) {
  const watched = Form.useWatch(path, { form, preserve: true });
  const lastWritten = useRef('');
  const serialize = (value: unknown) => JSON.stringify({ [wrapKey]: value ?? {} }, null, 2);
  const [text, setText] = useState(() => {
    const initial = serialize(form.getFieldValue(path));
    lastWritten.current = initial;
    return initial;
  });

  useEffect(() => {
    const next = serialize(watched);
    if (next === lastWritten.current) return;
    lastWritten.current = next;
    setText(next);
  }, [watched, wrapKey]);

  return (
    <Input.TextArea
      value={text}
      rows={rows}
      spellCheck={false}
      onChange={(event) => {
        const next = event.target.value;
        setText(next);
        try {
          const parsed = JSON.parse(next) as ListenerFormShape;
          const value = parsed && typeof parsed === 'object' && !Array.isArray(parsed)
            ? parsed[wrapKey] ?? {}
            : {};
          form.setFieldValue(path, value);
          lastWritten.current = JSON.stringify({ [wrapKey]: value }, null, 2);
        } catch {
          // Keep incomplete JSON in the editor until it becomes valid.
        }
      }}
    />
  );
}

export function AdvancedInboundAllEditor({ form, streamEnabled }: { form: FormInstance; streamEnabled: boolean }) {
  const bindHost = Form.useWatch('BindHost', { form, preserve: true });
  const bindPort = Form.useWatch('BindPort', { form, preserve: true });
  const protocol = Form.useWatch('Protocol', { form, preserve: true });
  const id = Form.useWatch('ID', { form, preserve: true });
  const binding = Form.useWatch('Binding', { form, preserve: true });
  const settings = Form.useWatch('settings', { form, preserve: true });
  const stream = Form.useWatch('streamSettings', { form, preserve: true });
  const lastWritten = useRef('');

  const serialize = () => JSON.stringify({
    listen: bindHost ?? '',
    port: bindPort ?? 0,
    protocol: protocol ?? '',
    tag: id ?? '',
    tapxBinding: binding ?? {},
    settings: settings ?? {},
    ...(streamEnabled ? { streamSettings: stream ?? {} } : {}),
  }, null, 2);

  const [text, setText] = useState(() => {
    const initial = serialize();
    lastWritten.current = initial;
    return initial;
  });

  useEffect(() => {
    const next = serialize();
    if (next === lastWritten.current) return;
    lastWritten.current = next;
    setText(next);
  }, [bindHost, bindPort, protocol, id, binding, settings, stream, streamEnabled]);

  return (
    <Input.TextArea
      value={text}
      rows={14}
      spellCheck={false}
      onChange={(event) => {
        const next = event.target.value;
        setText(next);
        let parsed: ListenerFormShape;
        try {
          parsed = JSON.parse(next) as ListenerFormShape;
        } catch {
          return;
        }
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return;
        if (typeof parsed.listen === 'string') form.setFieldValue('BindHost', parsed.listen);
        if (typeof parsed.port === 'number' && Number.isFinite(parsed.port)) form.setFieldValue('BindPort', parsed.port);
        if (typeof parsed.protocol === 'string') form.setFieldValue('Protocol', parsed.protocol);
        if (typeof parsed.tag === 'string') form.setFieldValue('ID', parsed.tag);
        if (isObject(parsed.tapxBinding)) form.setFieldValue('Binding', parsed.tapxBinding);
        if (isObject(parsed.settings)) form.setFieldValue('settings', parsed.settings);
        if (streamEnabled && isObject(parsed.streamSettings)) form.setFieldValue('streamSettings', parsed.streamSettings);
        lastWritten.current = next;
      }}
    />
  );
}

function isObject(value: unknown): value is ListenerFormShape {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}
