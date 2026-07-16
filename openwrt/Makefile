include $(TOPDIR)/rules.mk

PKG_NAME:=tapx
TAPX_SOURCE_VERSION:=$(if $(TAPX_VERSION),$(TAPX_VERSION),0.0.0-dev)
PKG_VERSION:=$(subst dev,git,$(subst -,_,$(TAPX_SOURCE_VERSION)))
PKG_RELEASE:=1
PKG_LICENSE:=MIT
PKG_MAINTAINER:=TapX

PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk

define Package/tapx-core
  SECTION:=net
  CATEGORY:=Network
  TITLE:=TapX TAP/TUN transport core
  DEPENDS:=+libc +kmod-tun +ip-full +tc-full +iptables-nft +ip6tables-nft +ca-bundle
endef

define Package/tapx-core/description
 Raw UDP/TCP and Xray TAP/TUN data plane for TapX.
endef

define Package/tapx-panel
  SECTION:=net
  CATEGORY:=Network
  TITLE:=TapX full Web panel
  DEPENDS:=+libc +tapx-core +ca-bundle
endef

define Package/tapx-panel/description
 TapX Web/API control plane with the same full UI used on Linux.
endef

define Package/luci-app-tapx
  SECTION:=luci
  CATEGORY:=LuCI
  SUBMENU:=3. Applications
  TITLE:=LuCI bridge for TapX
  PKGARCH:=all
  DEPENDS:=+luci-base +rpcd +tapx-core +tapx-panel
endef

define Package/luci-app-tapx/description
 Minimal LuCI setup, service control and log view for TapX.
endef

define Build/Prepare
	$(call Build/Prepare/Default)
	@test -x "$(TAPX_PREBUILT_DIR)/tapx-core" || \
		( echo "missing $(TAPX_PREBUILT_DIR)/tapx-core" >&2; exit 1 )
	@test -x "$(TAPX_PREBUILT_DIR)/tapx-panel" || \
		( echo "missing $(TAPX_PREBUILT_DIR)/tapx-panel" >&2; exit 1 )
	$(CP) "$(TAPX_PREBUILT_DIR)/tapx-core" $(PKG_BUILD_DIR)/tapx-core
	$(CP) "$(TAPX_PREBUILT_DIR)/tapx-panel" $(PKG_BUILD_DIR)/tapx-panel
endef

define Build/Compile
endef

define Package/tapx-core/conffiles
/etc/config/tapx
endef

define Package/tapx-core/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/tapx-core $(1)/usr/bin/tapx-core
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./tapx-core/files/etc/config/tapx $(1)/etc/config/tapx
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./tapx-core/files/etc/init.d/tapx $(1)/etc/init.d/tapx
	$(INSTALL_DIR) $(1)/etc/tapx
	$(INSTALL_DATA) ./tapx-core/files/etc/tapx/runtime.json.example $(1)/etc/tapx/runtime.json.example
endef

define Package/tapx-panel/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/tapx-panel $(1)/usr/bin/tapx-panel
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./tapx-panel/files/etc/init.d/tapx-panel $(1)/etc/init.d/tapx-panel
	$(INSTALL_DIR) $(1)/etc/tapx
	$(INSTALL_DIR) $(1)/lib/upgrade/keep.d
	$(INSTALL_DATA) ./tapx-panel/files/lib/upgrade/keep.d/tapx $(1)/lib/upgrade/keep.d/tapx
endef

define Package/luci-app-tapx/install
	$(INSTALL_DIR) $(1)/usr/libexec
	$(INSTALL_BIN) ./luci-app-tapx/root/usr/libexec/tapx-openwrt-config $(1)/usr/libexec/tapx-openwrt-config
	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./luci-app-tapx/root/usr/share/luci/menu.d/luci-app-tapx.json $(1)/usr/share/luci/menu.d/luci-app-tapx.json
	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./luci-app-tapx/root/usr/share/rpcd/acl.d/luci-app-tapx.json $(1)/usr/share/rpcd/acl.d/luci-app-tapx.json
	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/tapx
	$(INSTALL_DATA) ./luci-app-tapx/root/www/luci-static/resources/view/tapx/config.js $(1)/www/luci-static/resources/view/tapx/config.js
endef

$(eval $(call BuildPackage,tapx-core))
$(eval $(call BuildPackage,tapx-panel))
$(eval $(call BuildPackage,luci-app-tapx))
