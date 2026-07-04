import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { EditorView, basicSetup } from 'codemirror';
import { EditorState, Compartment } from '@codemirror/state';
import { json, jsonParseLinter } from '@codemirror/lang-json';
import { lintGutter, linter } from '@codemirror/lint';
import { oneDarkHighlightStyle } from '@codemirror/theme-one-dark';
import { syntaxHighlighting } from '@codemirror/language';
import { keymap } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';

import { useTheme } from '@/theme';
import './JsonEditor.css';

export interface JsonEditorProps {
  value: string;
  onChange?: (next: string) => void;
  minHeight?: string;
  maxHeight?: string;
  readOnly?: boolean;
}

export interface JsonEditorHandle {
  focus: () => void;
}

function buildDarkTheme(bg: string, panelBg: string, activeBg: string, border: string) {
  return EditorView.theme(
    {
      '&': { color: '#dcdcdc', backgroundColor: bg },
      '.cm-content': { caretColor: '#dcdcdc' },
      '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#dcdcdc' },
      '.cm-gutters': { backgroundColor: bg, borderRight: `1px solid ${border}`, color: '#6a6a6a' },
      '.cm-activeLine': { backgroundColor: activeBg },
      '.cm-activeLineGutter': { backgroundColor: activeBg, color: '#dcdcdc' },
      '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': { backgroundColor: '#3a3a3c' },
      '.cm-panels': { backgroundColor: panelBg, color: '#dcdcdc' },
      '.cm-tooltip': { backgroundColor: panelBg, border: `1px solid ${border}`, color: '#dcdcdc' },
    },
    { dark: true },
  );
}

function themeExtension(isDark: boolean, isUltra: boolean) {
  if (!isDark) return [];
  return [
    isUltra
      ? buildDarkTheme('#0a0a0a', '#141414', '#141414', '#1f1f1f')
      : buildDarkTheme('#1e1e1e', '#2d2d30', '#252526', '#3a3a3c'),
    syntaxHighlighting(oneDarkHighlightStyle),
  ];
}

export const JsonEditor = forwardRef<JsonEditorHandle, JsonEditorProps>(function JsonEditor(
  { value, onChange, minHeight = '320px', maxHeight = '600px', readOnly = false },
  ref,
) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const themeCompartmentRef = useRef(new Compartment());
  const readonlyCompartmentRef = useRef(new Compartment());
  const onChangeRef = useRef(onChange);
  const valueRef = useRef(value);
  const { isDark, isUltra } = useTheme();

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useImperativeHandle(ref, () => ({ focus: () => viewRef.current?.focus() }));

  useEffect(() => {
    if (!hostRef.current) return;
    const updateListener = EditorView.updateListener.of((u) => {
      if (!u.docChanged) return;
      const next = u.state.doc.toString();
      if (next === valueRef.current) return;
      valueRef.current = next;
      onChangeRef.current?.(next);
    });
    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          keymap.of([indentWithTab]),
          json(),
          linter(jsonParseLinter()),
          lintGutter(),
          EditorView.lineWrapping,
          updateListener,
          themeCompartmentRef.current.of(themeExtension(isDark, isUltra)),
          readonlyCompartmentRef.current.of(EditorState.readOnly.of(readOnly)),
          EditorView.theme({
            '&': { height: '100%' },
            '.cm-scroller': {
              fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
              fontSize: '12px',
              minHeight,
              maxHeight,
            },
          }),
        ],
      }),
    });
    viewRef.current = view;
    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, []);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (value === current) return;
    valueRef.current = value;
    view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
  }, [value]);

  useEffect(() => {
    viewRef.current?.dispatch({ effects: themeCompartmentRef.current.reconfigure(themeExtension(isDark, isUltra)) });
  }, [isDark, isUltra]);

  useEffect(() => {
    viewRef.current?.dispatch({ effects: readonlyCompartmentRef.current.reconfigure(EditorState.readOnly.of(readOnly)) });
  }, [readOnly]);

  return <div ref={hostRef} className="json-editor-host" aria-label="JSON editor" />;
});
