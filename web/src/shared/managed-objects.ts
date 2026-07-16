export const managedLinkAddressRemark = 'tapx:link-address-limit';
export const managedUserAddressRemark = 'tapx:user-address-limit';

// Keep recognizing records written by the legacy demo so existing databases remain editable.
const legacyManagedLinkAddressRemark = '\u94fe\u8def\u7ed1\u5b9a\u521b\u5efa\u7684\u6e90\u5730\u5740\u9650\u5236';
const legacyManagedUserAddressRemark = '\u7528\u6237\u9650\u5236\u521b\u5efa\u7684\u6e90\u5730\u5740\u9650\u5236';

export function isManagedLinkAddressRemark(remark?: string): boolean {
  return remark === managedLinkAddressRemark || remark === legacyManagedLinkAddressRemark;
}

export function isManagedUserAddressRemark(remark?: string): boolean {
  return remark === managedUserAddressRemark || remark === legacyManagedUserAddressRemark;
}
