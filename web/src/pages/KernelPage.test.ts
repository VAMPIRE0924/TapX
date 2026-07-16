import { describe, expect, it } from 'vitest';
import { diagnosticPlatform, officialXrayAsset, officialXrayDownloadURL } from './KernelPage';

describe('kernel download helpers', () => {
  it('uses the server platform instead of the browser platform', () => {
    expect(diagnosticPlatform({ process: { goos: 'linux', goarch: 'amd64' } })).toBe('linux-amd64');
  });

  it('maps supported Xray release assets', () => {
    expect(officialXrayAsset('linux-amd64')).toBe('Xray-linux-64.zip');
    expect(officialXrayAsset('linux-386')).toBe('Xray-linux-32.zip');
    expect(officialXrayAsset('windows-amd64')).toBe('Xray-windows-64.zip');
    expect(officialXrayAsset('freebsd-amd64')).toBe('');
  });

  it('builds latest and pinned official release URLs', () => {
    expect(officialXrayDownloadURL('linux-amd64')).toBe(
      'https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip',
    );
    expect(officialXrayDownloadURL('linux-amd64', '26.3.27')).toBe(
      'https://github.com/XTLS/Xray-core/releases/download/v26.3.27/Xray-linux-64.zip',
    );
  });
});
