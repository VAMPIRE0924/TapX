import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyText } from './clipboard';

const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
const originalDocument = Object.getOwnPropertyDescriptor(globalThis, 'document');

afterEach(() => {
  restoreGlobal('navigator', originalNavigator);
  restoreGlobal('document', originalDocument);
});

describe('copyText', () => {
  it('uses a confirmed browser copy event when available', async () => {
    let copyHandler: ((event: ClipboardEvent) => void) | undefined;
    const setData = vi.fn();
    const removeEventListener = vi.fn();
    defineGlobal('document', {
      addEventListener: vi.fn((_name: string, handler: (event: ClipboardEvent) => void) => { copyHandler = handler; }),
      removeEventListener,
      execCommand: vi.fn(() => {
        copyHandler?.({
          preventDefault: vi.fn(),
          clipboardData: { clearData: vi.fn(), setData },
        } as unknown as ClipboardEvent);
        return true;
      }),
    });
    const writeText = vi.fn();
    defineGlobal('navigator', { clipboard: { writeText } });

    await copyText('raw://tapx');

    expect(setData).toHaveBeenCalledWith('text/plain', 'raw://tapx');
    expect(removeEventListener).toHaveBeenCalledWith('copy', copyHandler, true);
    expect(writeText).not.toHaveBeenCalled();
  });

  it('uses the asynchronous Clipboard API when the browser event is unavailable', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    defineGlobal('document', undefined);
    defineGlobal('navigator', { clipboard: { writeText } });

    await copyText('vless://tapx');

    expect(writeText).toHaveBeenCalledWith('vless://tapx');
  });

  it('rejects when neither browser copy mechanism is available', async () => {
    defineGlobal('document', undefined);
    defineGlobal('navigator', {});

    await expect(copyText('tapx')).rejects.toThrow('clipboard is unavailable');
  });
});

function defineGlobal(name: 'navigator' | 'document', value: unknown) {
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
}

function restoreGlobal(name: 'navigator' | 'document', descriptor?: PropertyDescriptor) {
  if (descriptor) Object.defineProperty(globalThis, name, descriptor);
  else delete (globalThis as Record<string, unknown>)[name];
}
