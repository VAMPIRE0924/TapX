import { describe, expect, it } from 'vitest';
import { objectToSettings, settingsToObject, stableSettingsSnapshot } from './settings';

describe('typed panel settings adapter', () => {
  it('does not mark newly mounted undefined fields as changed', () => {
    expect(stableSettingsSnapshot({ listenPort: 2053, certPublicPath: undefined }))
      .toBe(stableSettingsSnapshot({ listenPort: 2053 }));
    expect(stableSettingsSnapshot({ listenPort: 2053, certPublicPath: '' }))
      .not.toBe(stableSettingsSnapshot({ listenPort: 2053 }));
  });

  it('keeps credential form values out of the global settings save', () => {
    const [row] = objectToSettings({
      settingsID: 'global',
      adminUsername: 'admin',
      _adminPasswordHash: 'stored-hash',
      oldUsername: 'old-form-value',
      oldPassword: 'old-secret',
      newUsername: 'new-form-value',
      newPassword: 'new-secret',
      listenPort: 2053,
    });

    expect(row.AdminUsername).toBe('admin');
    expect(row.AdminPasswordHash).toBe('stored-hash');
    expect(JSON.parse(row.AdvancedJSON || '{}')).not.toMatchObject({
      oldUsername: expect.anything(),
      oldPassword: expect.anything(),
      newUsername: expect.anything(),
      newPassword: expect.anything(),
    });
  });

  it('round-trips typed and advanced panel settings together', () => {
    const [row] = objectToSettings({
      settingsID: 'global',
      settingsName: 'TapX',
      listenIP: '127.0.0.1',
      listenPort: 2053,
      listenDomain: 'panel.example.com',
      uriPath: '/tapx/',
      sessionMinutes: 90,
      language: 'zh-CN',
      timezone: 'Asia/Hong_Kong',
      panelOutbound: 'connector-a',
      externalXrayPath: '/usr/local/bin/xray',
      externalXrayConfigFile: '/var/lib/tapx/xray.json',
      externalXrayWorkDir: '/var/lib/tapx',
      externalXrayArgs: 'run\n-config\n{config}',
      externalXrayEnabled: true,
      externalXrayReleaseChannel: 'latest',
      externalXrayTargetArch: 'linux-amd64',
      embeddedXrayEnabled: true,
      embeddedXrayPath: '/usr/local/bin/xray-embedded',
      tapxEnabled: true,
      tapxRuntimePath: '/usr/local/bin/tapx-core',
      tapxWorkerThreads: 4,
      defaultRuntimeMode: 'tapx',
    });

    expect(row).toMatchObject({
      PanelDomain: 'panel.example.com',
      PanelBasePath: '/tapx/',
      Timezone: 'Asia/Hong_Kong',
      PanelOutbound: 'connector-a',
      ExternalXrayPath: '/usr/local/bin/xray',
      ExternalXrayConfigFile: '/var/lib/tapx/xray.json',
      ExternalXrayWorkDir: '/var/lib/tapx',
      ExternalXrayArgs: 'run\n-config\n{config}',
    });
    expect(JSON.parse(row.AdvancedJSON || '{}')).not.toMatchObject({
      listenDomain: expect.anything(),
      uriPath: expect.anything(),
      timezone: expect.anything(),
      panelOutbound: expect.anything(),
      externalXrayPath: expect.anything(),
      externalXrayConfigFile: expect.anything(),
      externalXrayWorkDir: expect.anything(),
      externalXrayArgs: expect.anything(),
    });

    expect(settingsToObject([row])).toMatchObject({
      settingsID: 'global',
      settingsName: 'TapX',
      listenIP: '127.0.0.1',
      listenPort: 2053,
      listenDomain: 'panel.example.com',
      uriPath: '/tapx/',
      sessionMinutes: 90,
      language: 'zh-CN',
      timezone: 'Asia/Hong_Kong',
      panelOutbound: 'connector-a',
      externalXrayPath: '/usr/local/bin/xray',
      externalXrayConfigFile: '/var/lib/tapx/xray.json',
      externalXrayWorkDir: '/var/lib/tapx',
      externalXrayArgs: 'run\n-config\n{config}',
      externalXrayEnabled: true,
      externalXrayReleaseChannel: 'latest',
      externalXrayTargetArch: 'linux-amd64',
      embeddedXrayEnabled: true,
      embeddedXrayPath: '/usr/local/bin/xray-embedded',
      tapxEnabled: true,
      tapxRuntimePath: '/usr/local/bin/tapx-core',
      tapxWorkerThreads: 4,
      defaultRuntimeMode: 'tapx',
    });
  });
});
