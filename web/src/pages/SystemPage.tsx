import { useEffect, useState } from 'react';
import { Button, Card, Descriptions, Space, Table, Tabs, Tag, Typography, Upload, message } from 'antd';
import { ClearOutlined, DownloadOutlined, ReloadOutlined, UploadOutlined } from '@ant-design/icons';

import type { AnyRecord, RuntimeConfig } from '@/api';
import { apiURL, clearLogs, loadDiagnostics, loadLogs, saveConfig } from '@/api';
import { useI18n } from '@/i18n';
import { JsonEditor } from '@/components/JsonEditor';

interface SystemPageProps {
  config: RuntimeConfig;
  onRefresh: () => Promise<void>;
}

function exportURL(path: string) {
  window.location.href = apiURL(path);
}

export function SystemPage({ config, onRefresh }: SystemPageProps) {
  const { t } = useI18n();
  const [logs, setLogs] = useState<AnyRecord[]>([]);
  const [diagnostics, setDiagnostics] = useState<AnyRecord>({});
  const [configText, setConfigText] = useState(JSON.stringify(config, null, 2));

  useEffect(() => {
    setConfigText(JSON.stringify(config, null, 2));
  }, [config]);

  async function refreshLogs() {
    try {
      setLogs((await loadLogs()).events || []);
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  async function refreshDiagnostics() {
    try {
      setDiagnostics(await loadDiagnostics());
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  useEffect(() => {
    refreshLogs();
    refreshDiagnostics();
  }, []);

  async function importConfig(file: File) {
    try {
      const parsed = JSON.parse(await file.text());
      const next = parsed.config || parsed;
      await saveConfig(next);
      await onRefresh();
      message.success('restored');
    } catch (error) {
      message.error((error as Error).message);
    }
    return false;
  }

  async function saveConfigText() {
    try {
      await saveConfig(JSON.parse(configText));
      await onRefresh();
      message.success('saved');
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  const logTab = (
    <Card
      className="page-card"
      title={t('logs')}
      extra={(
        <Space>
          <Button icon={<ReloadOutlined />} onClick={refreshLogs}>{t('refresh')}</Button>
          <Button icon={<ClearOutlined />} onClick={async () => { await clearLogs(); await refreshLogs(); }}>{t('delete')}</Button>
        </Space>
      )}
    >
      <Table
        rowKey={(_, idx) => String(idx)}
        dataSource={logs}
        size="small"
        columns={[
          { title: 'Time', dataIndex: 'time', width: 220 },
          { title: 'Level', dataIndex: 'level', width: 100, render: (value) => <Tag>{value}</Tag> },
          { title: 'Action', dataIndex: 'action', width: 190 },
          { title: 'Message', dataIndex: 'message', render: (value) => <Typography.Text>{value}</Typography.Text> },
        ]}
      />
    </Card>
  );

  const backupTab = (
    <Card className="page-card" title={t('backup')}>
      <Space direction="vertical" className="full-width" size="middle">
        <Space wrap>
          <Button icon={<DownloadOutlined />} onClick={() => exportURL('/api/backup')}>{t('export')}</Button>
          <Upload beforeUpload={importConfig} showUploadList={false}>
            <Button icon={<UploadOutlined />}>{t('import')}</Button>
          </Upload>
          <Button type="primary" onClick={saveConfigText}>{t('save')}</Button>
        </Space>
        <JsonEditor value={configText} onChange={setConfigText} minHeight="520px" maxHeight="760px" />
      </Space>
    </Card>
  );

  const diagTab = (
    <Card
      className="page-card"
      title={t('diagnostics')}
      extra={<Button icon={<ReloadOutlined />} onClick={refreshDiagnostics}>{t('refresh')}</Button>}
    >
      <Descriptions bordered size="small" column={1}>
        {Object.entries(diagnostics).map(([key, value]) => (
          <Descriptions.Item key={key} label={key}>
            <Typography.Text code>{typeof value === 'object' ? JSON.stringify(value) : String(value)}</Typography.Text>
          </Descriptions.Item>
        ))}
      </Descriptions>
    </Card>
  );

  return (
    <Tabs
      className="page-tabs"
      items={[
        { key: 'logs', label: t('logs'), children: logTab },
        { key: 'backup', label: t('backup'), children: backupTab },
        { key: 'diagnostics', label: t('diagnostics'), children: diagTab },
      ]}
    />
  );
}
