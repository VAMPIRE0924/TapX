export const managedLinkAddressRemark = 'tapx:link-address-limit';
export const managedUserAddressRemark = 'tapx:user-address-limit';

export function isManagedLinkAddressRemark(remark?: string): boolean {
  return remark === managedLinkAddressRemark;
}

export function isManagedUserAddressRemark(remark?: string): boolean {
  return remark === managedUserAddressRemark;
}
