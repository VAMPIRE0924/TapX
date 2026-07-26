import type { TapxBinding } from './api';
import { nodeIDOf, type NodeOwned } from '../features/nodes/managedConfig';

type BoundObject = NodeOwned & { Binding?: TapxBinding };
type BindingReference = keyof Pick<TapxBinding, 'RouteID' | 'ConnectorID'>;

export function relationKey(record: object, id: string): string {
  return `${nodeIDOf(record as NodeOwned)}:${id}`;
}

export function clearBindingReferences<T extends BoundObject>(
  records: T[],
  field: BindingReference,
  removedKeys: ReadonlySet<string>,
): T[] {
  return records.map((record) => {
    const id = record.Binding?.[field];
    if (!id || !removedKeys.has(relationKey(record, id))) return record;
    return { ...record, Binding: { ...record.Binding, [field]: '' } };
  });
}
