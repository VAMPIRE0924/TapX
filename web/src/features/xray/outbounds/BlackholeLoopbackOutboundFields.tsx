import type { ReactNode } from 'react';
import { Form, Input, Select } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

export function BlackholeOutboundFields() {
  const { t } = useI18n();
  return (
    <Form.Item label={t('xray.responseType')} name={['settings', 'type']}>
      <Select
        options={[
          { value: '', label: '(empty)' },
          { value: 'none', label: 'none' },
          { value: 'http', label: 'http' },
        ]}
      />
    </Form.Item>
  );
}

export function LoopbackOutboundFields({ sniffing }: { sniffing: ReactNode }) {
  const { t } = useI18n();
  return (
    <>
      <Form.Item label={t('xray.inboundTag')} name={['settings', 'inboundTag']}>
        <Input placeholder={t('xray.loopbackTagPlaceholder')} />
      </Form.Item>
      {sniffing}
    </>
  );
}
