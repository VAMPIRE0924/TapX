import { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Col, Descriptions, Input, Row, Select, Space, Tabs, Upload, message } from 'antd';
import { CloudDownloadOutlined, ReloadOutlined, UploadOutlined } from '@ant-design/icons';

import type { AnyRecord, RuntimeConfig } from '@/api';
import { deepClone, downloadXrayBinary, loadXrayBinary, uploadXrayBinary } from '@/api';
import { kindByKey, type KindDef } from '@/schema';
import { useI18n } from '@/i18n';
import { ObjectListPage } from './ObjectListPage';
import { JsonEditor } from '@/components/JsonEditor';

interface XrayPageProps {
  config: RuntimeConfig;
  onSaveObject: (kind: KindDef, value: AnyRecord) => Promise<void>;
  onDeleteObject: (kind: KindDef, id: string) => Promise<void>;
  onReplaceConfig: (config: RuntimeConfig) => Promise<void>;
}

const officialAssets = [
  {
    label: 'Official latest Linux x86_64',
    value: 'https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip',
  },
  {
    label: 'Official latest Linux arm64',
    value: 'https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-arm64-v8a.zip',
  },
  {
    label: 'Official latest OpenWrt x86_64',
    value: 'https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip',
  },
];

function firstSettings(config: RuntimeConfig) {
  const settings = Array.isArray(config.Settings) ? config.Settings : [];
  return settings.find((item) => item.Enabled) || settings[0] || null;
}

export function XrayPage({ config, onSaveObject, onDeleteObject, onReplaceConfig }: XrayPageProps) {
  const { t } = useI18n();
  const xrayKind = kindByKey.xrayProfiles;
  const settingsKind = kindByKey.settings;
  const settings = firstSettings(config);
  const [path, setPath] = useState(settings?.ExternalXrayPath || '/usr/local/bin/xray');
  const [url, setUrl] = useState(officialAssets[0].value);
  const [binary, setBinary] = useState<AnyRecord | null>(null);
  const [templateText, setTemplateText] = useState(JSON.stringify(config.XrayProfiles || [], null, 2));
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setPath(settings?.ExternalXrayPath || '/usr/local/bin/xray');
  }, [settings?.ExternalXrayPath]);

  useEffect(() => {
    setTemplateText(JSON.stringify(config.XrayProfiles || [], null, 2));
  }, [config.XrayProfiles]);

  async function refreshBinary(nextPath = path) {
    try {
      setBinary((await loadXrayBinary(nextPath)).binary);
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  useEffect(() => {
    refreshBinary();
  }, []);

  async function savePath() {
    const next = deepClone(settings || settingsKind.template('default'));
    next.ExternalXrayPath = path;
    await onSaveObject(settingsKind, next);
    await refreshBinary(path);
    message.success('path saved');
  }

  async function download() {
    setBusy(true);
    try {
      const response = await downloadXrayBinary(url, path);
      setBinary(response.binary);
      message.success('downloaded');
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function upload(file: File) {
    setBusy(true);
    try {
      const response = await uploadXrayBinary(file, path);
      setBinary(response.binary);
      message.success('uploaded');
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setBusy(false);
    }
    return false;
  }

  async function saveTemplate() {
    try {
      const parsed = JSON.parse(templateText);
      if (!Array.isArray(parsed)) throw new Error('Xray profile JSON must be an array');
      await onReplaceConfig({ ...deepClone(config), XrayProfiles: parsed });
      message.success('saved');
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  const binaryTab = (
    <Space direction="vertical" size="middle" className="full-width">
      <Alert
        type="info"
        showIcon
        message="External xray-core can use an existing local binary path, or download/upload a binary into that path."
      />
      <Card className="page-card" title="Xray Binary">
        <Space direction="vertical" className="full-width" size="middle">
          <Row gutter={[12, 12]}>
            <Col xs={24} lg={14}>
              <Input value={path} onChange={(event) => setPath(event.target.value)} addonBefore="Path" />
            </Col>
            <Col xs={24} lg={10}>
              <Space wrap>
                <Button onClick={savePath}>{t('save')}</Button>
                <Button icon={<ReloadOutlined />} onClick={() => refreshBinary()}>{t('refresh')}</Button>
              </Space>
            </Col>
          </Row>
          <Row gutter={[12, 12]}>
            <Col xs={24} lg={14}>
              <Select className="full-width" value={url} options={officialAssets} onChange={setUrl} />
            </Col>
            <Col xs={24} lg={10}>
              <Space wrap>
                <Button type="primary" loading={busy} icon={<CloudDownloadOutlined />} onClick={download}>{t('download')}</Button>
                <Upload beforeUpload={upload} showUploadList={false}>
                  <Button loading={busy} icon={<UploadOutlined />}>{t('upload')}</Button>
                </Upload>
              </Space>
            </Col>
          </Row>
          <Input value={url} onChange={(event) => setUrl(event.target.value)} addonBefore="URL" />
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="Path">{binary?.path || path}</Descriptions.Item>
            <Descriptions.Item label="Exists">{String(Boolean(binary?.exists))}</Descriptions.Item>
            <Descriptions.Item label="Executable">{String(Boolean(binary?.executable))}</Descriptions.Item>
            <Descriptions.Item label="Size">{binary?.size || 0}</Descriptions.Item>
            <Descriptions.Item label="Mode">{binary?.mode || ''}</Descriptions.Item>
            <Descriptions.Item label="Modified">{binary?.modifiedAt || ''}</Descriptions.Item>
            <Descriptions.Item label="Error">{binary?.error || ''}</Descriptions.Item>
          </Descriptions>
        </Space>
      </Card>
    </Space>
  );

  const templateTab = (
    <Card className="page-card" title="Xray Profile JSON">
      <Space direction="vertical" className="full-width" size="middle">
        <JsonEditor value={templateText} onChange={setTemplateText} minHeight="520px" maxHeight="760px" />
        <Button type="primary" onClick={saveTemplate}>{t('save')}</Button>
      </Space>
    </Card>
  );

  const tabs = useMemo(() => [
    {
      key: 'profiles',
      label: 'Profiles',
      children: (
        <ObjectListPage
          kind={xrayKind}
          config={config}
          onSaveObject={onSaveObject}
          onDeleteObject={onDeleteObject}
          onReplaceConfig={onReplaceConfig}
        />
      ),
    },
    { key: 'binary', label: t('xrayBinary'), children: binaryTab },
    { key: 'template', label: 'Template JSON', children: templateTab },
  ], [binaryTab, config, onDeleteObject, onReplaceConfig, onSaveObject, templateTab, t, xrayKind]);

  return <Tabs className="page-tabs" defaultActiveKey={window.location.hash.replace('#', '') || 'profiles'} items={tabs} />;
}
