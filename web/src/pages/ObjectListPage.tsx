import { useMemo, useState } from 'react';
import { Button, Card, Dropdown, Space, Table, Tag, Typography, Upload, message, Modal } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { MenuProps } from 'antd';
import {
  CopyOutlined,
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  UploadOutlined,
} from '@ant-design/icons';

import type { AnyRecord, RuntimeConfig } from '@/api';
import { deepClone, loadClientShare, resetClientTraffic } from '@/api';
import { generatedID, getItems, getPath, type KindDef } from '@/schema';
import { useI18n } from '@/i18n';
import { ObjectEditor } from '@/components/ObjectEditor';

interface ObjectListPageProps {
  kind: KindDef;
  config: RuntimeConfig;
  onSaveObject: (kind: KindDef, value: AnyRecord) => Promise<void>;
  onDeleteObject: (kind: KindDef, id: string) => Promise<void>;
  onReplaceConfig: (config: RuntimeConfig) => Promise<void>;
}

function exportText(name: string, text: string) {
  const link = document.createElement('a');
  link.download = name;
  link.href = URL.createObjectURL(new Blob([text], { type: 'application/json' }));
  link.click();
  URL.revokeObjectURL(link.href);
}

function compact(value: unknown) {
  if (Array.isArray(value)) return value.join(', ');
  if (value && typeof value === 'object') return JSON.stringify(value);
  return value == null ? '' : String(value);
}

export function ObjectListPage({ kind, config, onSaveObject, onDeleteObject, onReplaceConfig }: ObjectListPageProps) {
  const { t } = useI18n();
  const [editing, setEditing] = useState<AnyRecord | null>(null);
  const [modal, contextHolder] = Modal.useModal();
  const items = getItems(config, kind);

  const columns = useMemo<ColumnsType<AnyRecord>>(() => {
    const cols: ColumnsType<AnyRecord> = [
      {
        title: t('enabled'),
        dataIndex: 'Enabled',
        width: 90,
        render: (value) => <Tag color={value ? 'green' : 'default'}>{value ? 'on' : 'off'}</Tag>,
      },
      {
        title: 'ID',
        dataIndex: 'ID',
        width: 180,
        fixed: 'left',
        render: (value, row) => (
          <Space direction="vertical" size={0}>
            <Typography.Text copyable strong>{value}</Typography.Text>
            {row.Name && <Typography.Text type="secondary">{row.Name}</Typography.Text>}
          </Space>
        ),
      },
      ...kind.primaryFields.map((field) => ({
        title: field,
        dataIndex: field,
        ellipsis: true,
        render: (_: unknown, row: AnyRecord) => compact(getPath(row, field)),
      })),
      {
        title: 'Remark',
        dataIndex: 'Remark',
        ellipsis: true,
      },
      {
        title: '',
        key: 'actions',
        width: 74,
        align: 'right',
        render: (_, row) => {
          const menu: MenuProps['items'] = [
            { key: 'edit', icon: <EditOutlined />, label: t('edit') },
            { key: 'clone', icon: <CopyOutlined />, label: t('clone') },
            { key: 'export', icon: <DownloadOutlined />, label: t('export') },
            ...(kind.key === 'clients' ? [
              { key: 'share', icon: <DownloadOutlined />, label: 'Client Share' },
              { key: 'reset-traffic', icon: <ReloadOutlined />, label: 'Reset Traffic' },
            ] : []),
            { type: 'divider' },
            { key: 'delete', icon: <DeleteOutlined />, label: t('delete'), danger: true },
          ];
          return (
            <Dropdown
              menu={{
                items: menu,
                onClick: ({ key }) => {
                  if (key === 'edit') setEditing(deepClone(row));
                  if (key === 'clone') {
                    const clone = deepClone(row);
                    clone.ID = generatedID(kind);
                    clone.Enabled = false;
                    setEditing(clone);
                  }
                  if (key === 'export') exportText(`${row.ID || kind.key}.json`, JSON.stringify(row, null, 2));
                  if (key === 'share') {
                    loadClientShare(row.ID)
                      .then((payload) => {
                        modal.info({
                          title: 'Client Share',
                          width: 760,
                          content: <pre className="modal-pre">{JSON.stringify(payload.share, null, 2)}</pre>,
                        });
                      })
                      .catch((error) => message.error((error as Error).message));
                  }
                  if (key === 'reset-traffic') {
                    resetClientTraffic(row.ID)
                      .then(() => message.success('traffic reset'))
                      .catch((error) => message.error((error as Error).message));
                  }
                  if (key === 'delete') {
                    modal.confirm({
                      title: `Delete ${row.ID}?`,
                      okText: t('delete'),
                      okType: 'danger',
                      onOk: () => onDeleteObject(kind, row.ID),
                    });
                  }
                },
              }}
            >
              <Button icon={<MoreOutlined />} />
            </Dropdown>
          );
        },
      },
    ];
    return cols;
  }, [kind, modal, onDeleteObject, t]);

  async function importObject(file: File) {
    try {
      const text = await file.text();
      const parsed = JSON.parse(text);
      if (Array.isArray(parsed)) {
        const next = deepClone(config);
        next[kind.configKey] = parsed;
        await onReplaceConfig(next);
      } else {
        setEditing(parsed);
      }
      message.success('imported');
    } catch (error) {
      message.error((error as Error).message);
    }
    return false;
  }

  return (
    <>
      {contextHolder}
      <Card
        className="page-card"
        title={(
          <Space direction="vertical" size={0}>
            <span>{kind.title}</span>
            <Typography.Text type="secondary">{kind.summary}</Typography.Text>
          </Space>
        )}
        extra={(
          <Space wrap>
            <Upload beforeUpload={importObject} showUploadList={false}>
              <Button icon={<UploadOutlined />}>{t('import')}</Button>
            </Upload>
            <Button icon={<DownloadOutlined />} onClick={() => exportText(`${kind.key}.json`, JSON.stringify(items, null, 2))}>
              {t('export')}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing(kind.template(generatedID(kind)))}>
              {t('add')}
            </Button>
          </Space>
        )}
      >
        <Table
          rowKey={(row) => row.ID}
          dataSource={items}
          columns={columns}
          size="middle"
          scroll={{ x: 960 }}
          pagination={{ pageSize: 12, showSizeChanger: true }}
          locale={{ emptyText: t('noData') }}
        />
      </Card>
      {editing && (
        <ObjectEditor
          open={!!editing}
          kind={kind}
          value={editing}
          onClose={() => setEditing(null)}
          onSave={async (value) => {
            await onSaveObject(kind, value);
            setEditing(null);
          }}
        />
      )}
    </>
  );
}
