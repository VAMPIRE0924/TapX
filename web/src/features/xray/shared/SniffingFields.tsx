import { Form, Select, Switch, type FormInstance } from 'antd';
import { useI18n } from '../../../i18n/I18nProvider';

const destOptions = ['http', 'tls', 'quic', 'fakedns'].map((value) => ({ value, label: value }));

export function SniffingFields({
  form,
  name,
  label,
}: {
  form: FormInstance;
  name: Array<string | number>;
  label: string;
}) {
  const { t } = useI18n();
  const enabled = Form.useWatch([...name, 'enabled'], form) === true;

  return (
    <>
      <Form.Item label={label} name={[...name, 'enabled']} tooltip={t('xray.sniffingHelp')} valuePropName="checked">
        <Switch />
      </Form.Item>
      {enabled ? (
        <>
          <Form.Item label="Destination Override" name={[...name, 'destOverride']} tooltip={t('xray.destOverrideHelp')}>
            <Select mode="multiple" options={destOptions} />
          </Form.Item>
          <Form.Item label="Metadata Only" name={[...name, 'metadataOnly']} tooltip={t('xray.metadataOnlyHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="Route Only" name={[...name, 'routeOnly']} tooltip={t('xray.routeOnlyHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label={t('xray.excludeIp')} name={[...name, 'ipsExcluded']} tooltip={t('xray.excludeIpHelp')}>
            <Select mode="tags" tokenSeparators={[',']} placeholder="IP/CIDR/geoip:*/ext:*" />
          </Form.Item>
          <Form.Item label={t('xray.excludeDomains')} name={[...name, 'domainsExcluded']} tooltip={t('xray.excludeDomainsHelp')}>
            <Select mode="tags" tokenSeparators={[',']} placeholder="domain:*/ext:*" />
          </Form.Item>
        </>
      ) : null}
    </>
  );
}
