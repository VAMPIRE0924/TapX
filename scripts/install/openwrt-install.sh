#!/bin/sh
set -eu

package_dir="${1:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)}"

need_root() {
	[ "$(id -u)" = "0" ] || {
		echo "请使用 root 运行安装脚本" >&2
		exit 1
	}
}

collect_packages() {
	extension="$1"
	set -- \
		"$package_dir"/tapx-core*.$extension \
		"$package_dir"/tapx-panel*.$extension \
		"$package_dir"/luci-app-tapx*.$extension
	for file in "$@"; do
		[ -f "$file" ] || {
			echo "缺少 TapX .$extension 安装包：$package_dir" >&2
			exit 1
		}
	done
	printf '%s\n' "$@"
}

need_root

if command -v apk >/dev/null 2>&1; then
	packages="$(collect_packages apk)"
	apk update || echo "警告：软件源索引刷新失败，继续尝试安装本地 TapX 软件包。" >&2
	# Package dependencies are resolved from the configured OpenWrt repositories.
	# shellcheck disable=SC2086
	apk add --allow-untrusted --force-reinstall $packages
elif command -v opkg >/dev/null 2>&1; then
	packages="$(collect_packages ipk)"
	opkg update || echo "警告：软件源索引刷新失败，继续尝试安装本地 TapX 软件包。" >&2
	# shellcheck disable=SC2086
	opkg install --force-reinstall $packages
else
	echo "未找到 OpenWrt 包管理器 apk 或 opkg" >&2
	exit 1
fi

/etc/init.d/rpcd restart 2>/dev/null || true
/etc/init.d/uhttpd restart 2>/dev/null || true

cat <<'EOF'
TapX 已安装，服务尚未启动。
请进入 LuCI -> 服务 -> TapX 完成首次设置。
EOF
