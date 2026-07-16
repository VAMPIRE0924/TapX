import { useEffect, useMemo, useState } from 'react';
import { ClusterOutlined, HomeOutlined } from '@ant-design/icons';
import { Select, Space, Tag } from 'antd';
import { useI18n } from '../../i18n/I18nProvider';
import { ALL_NODE_ID, LOCAL_NODE_ID, nodeIDOf, type NodeOwned } from './managedConfig';
import { loadManagedNodes, nodeRegistryChangedEvent, readManagedNodes, type ManagedNode } from './nodeRegistry';
import './NodeScope.css';

const scopeStorageKey = 'tapx-managed-node-scope-v1';

export function useNodeScope() {
  const [nodes, setNodes] = useState<ManagedNode[]>(readManagedNodes);
  const [scope, setScopeState] = useState(() => readScope());

  useEffect(() => {
    const refresh = () => setNodes(readManagedNodes());
    void loadManagedNodes().then(setNodes).catch(() => setNodes(readManagedNodes()));
    window.addEventListener(nodeRegistryChangedEvent, refresh);
    return () => {
      window.removeEventListener(nodeRegistryChangedEvent, refresh);
    };
  }, []);

  useEffect(() => {
    if (scope !== ALL_NODE_ID && scope !== LOCAL_NODE_ID && !nodes.some((node) => node.ID === scope && node.Enabled)) {
      setScopeState(ALL_NODE_ID);
    }
  }, [nodes, scope]);

  const setScope = (value: string) => {
    setScopeState(value);
    try {
      window.localStorage.setItem(scopeStorageKey, value);
    } catch {
      // Scope still applies for the current page.
    }
  };

  return { nodes, scope, setScope };
}

export function NodeScopeSelect({ scope, onChange }: { scope: string; onChange: (value: string) => void }) {
  const { t } = useI18n();
  const nodes = readManagedNodes();
  return (
    <Space size={8} className="node-scope-select">
      <ClusterOutlined />
      <span>{t('node.scope')}</span>
      <Select
        value={scope}
        onChange={onChange}
        popupMatchSelectWidth={false}
        options={[
          { value: ALL_NODE_ID, label: t('node.scopeAll') },
          { value: LOCAL_NODE_ID, label: t('node.localPanel') },
          ...nodes.map((node) => ({ value: node.ID, label: node.Name, disabled: !node.Enabled })),
        ]}
      />
    </Space>
  );
}

export function useNodeTargetOptions(nodes: ManagedNode[]) {
  const { t } = useI18n();
  return useMemo(() => [
    { value: LOCAL_NODE_ID, label: t('node.localPanel') },
    ...nodes.map((node) => ({ value: node.ID, label: node.Name, disabled: !node.Enabled })),
  ], [nodes, t]);
}

export function NodeSourceTag({ value }: { value: NodeOwned | null | undefined }) {
  const { t } = useI18n();
  const nodeID = nodeIDOf(value);
  if (nodeID === LOCAL_NODE_ID) {
    return <Tag icon={<HomeOutlined />} color="blue">{t('node.localPanel')}</Tag>;
  }
  const node = readManagedNodes().find((item) => item.ID === nodeID);
  return <Tag icon={<ClusterOutlined />} color="cyan">{node?.Name || nodeID}</Tag>;
}

function readScope(): string {
  if (typeof window === 'undefined') return ALL_NODE_ID;
  try {
    return window.localStorage.getItem(scopeStorageKey) || ALL_NODE_ID;
  } catch {
    return ALL_NODE_ID;
  }
}
