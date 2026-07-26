import { describe, expect, it } from 'vitest';

import { runtimeModeChangesTransportFamily } from './endpoint-types';

describe('runtimeModeChangesTransportFamily', () => {
  it('preserves endpoint fields when only the Xray process changes', () => {
    expect(runtimeModeChangesTransportFamily('embedded-xray', 'external-xray')).toBe(false);
    expect(runtimeModeChangesTransportFamily('external-xray', 'embedded-xray')).toBe(false);
  });

  it('resets family-specific fields when moving between Xray and TapX raw transport', () => {
    expect(runtimeModeChangesTransportFamily('embedded-xray', 'tapx')).toBe(true);
    expect(runtimeModeChangesTransportFamily('tapx', 'external-xray')).toBe(true);
  });
});
