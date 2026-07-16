import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Collapse, Modal, Radio, Spin, Switch, Tag, message } from 'antd';
import { CloudDownloadOutlined } from '@ant-design/icons';
import {
  getComponentUpdateCatalog,
  installComponentUpdate,
  type ComponentUpdateCatalog,
  type UpdateComponent,
} from '../../shared/api';
import { errorMessage } from '../../shared/localized-error';
import { useI18n } from '../../i18n/I18nProvider';
import './ComponentUpdateDialog.css';

interface ComponentUpdateDialogProps {
  open: boolean;
  component: UpdateComponent;
  externalXrayPath?: string;
  onClose: () => void;
  onUpdated?: () => void;
}

export function ComponentUpdateDialog({
  open,
  component,
  externalXrayPath,
  onClose,
  onUpdated,
}: ComponentUpdateDialogProps) {
  const { t } = useI18n();
  const [catalog, setCatalog] = useState<ComponentUpdateCatalog>();
  const [development, setDevelopment] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState('');
  const [loading, setLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();

  const load = useCallback(async (nextDevelopment = development) => {
    setLoading(true);
    try {
      const next = await getComponentUpdateCatalog(component, {
        channel: nextDevelopment ? 'development' : 'stable',
        path: component === 'external-xray' ? externalXrayPath : undefined,
      });
      setCatalog(next);
      const preferred = next.versions.find((item) => item.latest && item.installable)
        || next.versions.find((item) => item.current)
        || next.versions[0];
      setSelectedVersion(preferred?.version || '');
    } catch (err) {
      setCatalog(undefined);
      messageApi.error(errorMessage(err, t, 'update.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [component, development, externalXrayPath, messageApi, t]);

  useEffect(() => {
    if (!open) return;
    setDevelopment(false);
    void load(false);
  }, [component, open]);

  const selected = useMemo(
    () => catalog?.versions.find((item) => item.version === selectedVersion),
    [catalog, selectedVersion],
  );
  const latest = catalog?.versions.find((item) => item.latest);
  const current = catalog?.versions.find((item) => item.current);
  const canInstall = Boolean(selected?.installable && selectedVersion);

  function changeChannel(checked: boolean) {
    setDevelopment(checked);
    void load(checked);
  }

  function confirmInstall() {
    if (!canInstall || !selectedVersion) return;
    modal.confirm({
      title: t('update.confirmTitle'),
      content: t('update.confirmDescription', { component: componentTitle(component, t), version: selectedVersion }),
      okText: t('common.update'),
      cancelText: t('common.cancel'),
      onOk: async () => {
        setInstalling(true);
        try {
          const result = await installComponentUpdate(component, selectedVersion, externalXrayPath);
          messageApi.success(t('update.completed'));
          if (result.runtimeWarning) messageApi.warning(t('update.runtimeRestartFailed'));
          if (result.restarting) {
            onClose();
            window.setTimeout(() => window.location.reload(), 2500);
            return;
          }
          await load(development);
          onUpdated?.();
        } catch (err) {
          messageApi.error(errorMessage(err, t, 'update.failed'));
          throw err;
        } finally {
          setInstalling(false);
        }
      },
    });
  }

  return (
    <>
      {messageContextHolder}
      {modalContextHolder}
      <Modal
        className="component-update-modal"
        open={open}
        title={component === 'panel' ? t('update.panelTitle') : t('update.kernelTitle', { component: componentTitle(component, t) })}
        footer={null}
        onCancel={onClose}
        destroyOnHidden
      >
        <Spin spinning={loading}>
          {component === 'panel' ? (
            <>
              <div className="update-version-list">
                <div className="update-version-row">
                  <span>{t('update.developmentChannel')}</span>
                  <Switch checked={development} onChange={changeChannel} />
                </div>
              </div>
              {development ? <Alert type="info" showIcon title={t('update.developmentWarning')} /> : null}
              <div className="update-version-list">
                <VersionSummary label={t('update.currentPanelVersion')} version={current?.version || catalog?.currentVersion} color="green" />
                {latest && !latest.current ? (
                  <VersionSummary label={t('update.latestPanelVersion')} version={latest.version} color="purple" />
                ) : (
                  <div className="update-version-row">
                    <span>{t('update.upToDate')}</span>
                    <Tag color="green">{t('update.upToDate')}</Tag>
                  </div>
                )}
              </div>
            </>
          ) : (
            <>
              {component === 'tapx' ? (
                <div className="update-version-list">
                  <VersionSummary label="TapX" version={catalog?.currentVersion} color="green" />
                  <VersionSummary label={t('dashboard.embeddedXray')} version={catalog?.relatedVersions?.embeddedXray} color="green" />
                </div>
              ) : null}
              <Collapse
                defaultActiveKey={['versions']}
                items={[{
                key: 'versions',
                label: componentTitle(component, t),
                children: (
                  <>
                    <Alert type="warning" showIcon title={t('update.versionWarning')} />
                    <div className="update-version-list">
                      {(catalog?.versions || []).map((item, index) => (
                        <label className="update-version-row" key={item.version}>
                          <span className="update-version-tags">
                            <Tag color={index % 2 === 0 ? 'purple' : 'green'}>{formatVersionLabel(item.version)}</Tag>
                            {item.current ? <Tag>{t('update.current')}</Tag> : null}
                            {item.latest ? <Tag color="blue">{t('update.latest')}</Tag> : null}
                          </span>
                          <Radio
                            checked={selectedVersion === item.version}
                            onChange={() => setSelectedVersion(item.version)}
                          />
                        </label>
                      ))}
                    </div>
                  </>
                ),
                }]}
              />
            </>
          )}

          {catalog?.message ? (
            <Alert
              className="update-delivery-note"
              type={component === 'external-xray' ? 'info' : 'warning'}
              showIcon
              title={deliveryMessage(component, t)}
            />
          ) : null}

          {catalog && !catalog.installReady ? (
            <Alert
              type="warning"
              showIcon
              title={component === 'external-xray' ? t('update.externalPathRequired') : t('update.platformUnavailable')}
            />
          ) : null}

          <div className="update-meta">
            <span>{t('update.source')}: {catalog?.source || '-'}</span>
            <span>{t('update.platform')}: {catalog?.platform || '-'}</span>
          </div>

          <div className="update-actions">
            <Button onClick={onClose}>{t('common.cancel')}</Button>
            <Button
              type="primary"
              icon={<CloudDownloadOutlined />}
              loading={installing}
              disabled={!canInstall}
              onClick={confirmInstall}
            >
              {t('common.update')}
            </Button>
          </div>
        </Spin>
      </Modal>
    </>
  );
}

function VersionSummary({ label, version, color }: { label: string; version?: string; color: 'green' | 'purple' }) {
  return (
    <div className="update-version-row">
      <span>{label}</span>
      <Tag color={color}>{formatVersionLabel(version)}</Tag>
    </div>
  );
}

function formatVersionLabel(version?: string): string {
  if (!version) return '?';
  return version === 'dev' || version === 'unknown' ? version : `v${version.replace(/^v/, '')}`;
}

function componentTitle(component: UpdateComponent, t: ReturnType<typeof useI18n>['t']): string {
  if (component === 'panel') return 'TapX-UI';
  if (component === 'tapx') return t('update.tapxCore');
  if (component === 'embedded-xray') return t('update.tapxCore');
  return t('dashboard.externalXray');
}

function deliveryMessage(component: UpdateComponent, t: ReturnType<typeof useI18n>['t']): string {
  if (component === 'external-xray') return t('update.externalDelivery');
  if (component === 'embedded-xray' || component === 'tapx') return t('update.embeddedDelivery');
  return t('update.bundleDelivery');
}
