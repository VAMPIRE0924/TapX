.PHONY: all test panel-web go-test fastpath verify-local verify-release build-linux build-linux-amd64 build-linux-arm64 build-openwrt-x86 package-openwrt-x86 package-release install-linux integration-netns integration-netns-full integration-device-apply-netns integration-bridge-apply-netns integration-mss-clamp-netns integration-dns-apply-netns integration-tun-netns integration-tun-ipv6-underlay-netns integration-tun-vkey-netns integration-udp-vkey-route-netns integration-udp-dtls-vkey-route-netns integration-tap-netns integration-tap-l2-transparent-netns integration-tap-l2-udp-dtls-netns integration-tap-l2-tcp-netns integration-tap-l2-tcp-tls-netns integration-tap-l2-xray-netns integration-tap-l2-xray-external-netns integration-xray-mkcp-ipv6-underlay-netns integration-xray-user-route-netns integration-tcp-tun-netns integration-tcp-tun-ipv6-underlay-netns integration-tcp-vkey-route-netns integration-tcp-tls-vkey-route-netns integration-tcp-tls-tun-netns integration-udp-dtls-tun-netns integration-address-guard-netns integration-user-rate-limit-netns integration-user-rate-limit-xray-netns integration-user-rate-limit-xray-external-netns clean env

CC ?= gcc
CFLAGS ?= -std=c11 -O3 -Wall -Wextra -Werror -fPIC

all: test

env:
	bash ./scripts/dev/check-env.sh

test: go-test fastpath

panel-web:
	cd web && npm ci && npm run typecheck && npm test && npm run build
	node ./scripts/build/sync-panel-web.mjs

go-test:
	go test ./...

fastpath:
	$(MAKE) -C core/fastpath test

verify-local:
	go run ./scripts/verify

verify-release:
	go run ./scripts/verify -require-openwrt-package

build-linux: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	bash ./scripts/build/linux-amd64.sh

build-linux-arm64:
	bash ./scripts/build/linux-arm64.sh

build-openwrt-x86:
	bash ./scripts/build/openwrt-x86-64.sh

package-openwrt-x86: panel-web build-openwrt-x86
	bash ./scripts/build/openwrt-x86-64-packages.sh

package-release: build-linux package-openwrt-x86
	bash ./scripts/build/release-archives.sh

install-linux: build-linux-amd64
	bash ./scripts/install/linux-install.sh

integration-netns: integration-device-apply-netns integration-bridge-apply-netns integration-mss-clamp-netns integration-dns-apply-netns integration-tun-netns integration-tun-ipv6-underlay-netns integration-tun-vkey-netns integration-udp-vkey-route-netns integration-udp-dtls-vkey-route-netns integration-tap-netns integration-tap-l2-transparent-netns integration-tcp-tun-netns integration-tcp-tun-ipv6-underlay-netns integration-tcp-vkey-route-netns integration-tcp-tls-vkey-route-netns integration-tcp-tls-tun-netns integration-udp-dtls-tun-netns integration-address-guard-netns integration-user-rate-limit-netns

integration-netns-full: integration-netns integration-tap-l2-udp-dtls-netns integration-tap-l2-tcp-netns integration-tap-l2-tcp-tls-netns integration-tap-l2-xray-netns integration-tap-l2-xray-external-netns integration-xray-mkcp-ipv6-underlay-netns integration-xray-user-route-netns integration-user-rate-limit-xray-netns integration-user-rate-limit-xray-external-netns

integration-device-apply-netns:
	bash ./scripts/integration/device-apply-netns.sh

integration-bridge-apply-netns:
	bash ./scripts/integration/bridge-apply-netns.sh

integration-mss-clamp-netns:
	bash ./scripts/integration/mss-clamp-netns.sh

integration-dns-apply-netns:
	bash ./scripts/integration/dns-apply-netns.sh

integration-tun-netns:
	bash ./scripts/integration/raw-udp-tun-netns.sh

integration-tun-ipv6-underlay-netns:
	bash ./scripts/integration/raw-udp-ipv6-underlay-netns.sh

integration-tun-vkey-netns:
	bash ./scripts/integration/raw-udp-tun-vkey-netns.sh

integration-udp-vkey-route-netns:
	bash ./scripts/integration/raw-udp-vkey-route-netns.sh

integration-udp-dtls-vkey-route-netns:
	SECURITY=dtls bash ./scripts/integration/raw-udp-vkey-route-netns.sh

integration-tap-netns:
	bash ./scripts/integration/raw-udp-tap-netns.sh

integration-tap-l2-transparent-netns:
	TRANSPORT=udp bash ./scripts/integration/tap-l2-transparent-netns.sh

integration-tap-l2-udp-dtls-netns:
	TRANSPORT=udp-dtls bash ./scripts/integration/tap-l2-transparent-netns.sh

integration-tap-l2-tcp-netns:
	TRANSPORT=tcp bash ./scripts/integration/tap-l2-transparent-netns.sh

integration-tap-l2-tcp-tls-netns:
	TRANSPORT=tcp-tls bash ./scripts/integration/tap-l2-transparent-netns.sh

integration-tap-l2-xray-netns:
	TRANSPORT=xray bash ./scripts/integration/tap-l2-transparent-netns.sh

integration-tap-l2-xray-external-netns:
	mkdir -p ./build/lab
	CGO_ENABLED=0 go build -trimpath -o ./build/lab/xray-linux-amd64 github.com/xtls/xray-core/main
	TRANSPORT=xray-external XRAY_BIN="$$(pwd)/build/lab/xray-linux-amd64" bash ./scripts/integration/tap-l2-transparent-netns.sh

integration-xray-mkcp-ipv6-underlay-netns:
	bash ./scripts/integration/xray-mkcp-ipv6-underlay-netns.sh

integration-xray-user-route-netns:
	bash ./scripts/integration/xray-user-route-netns.sh embedded separate
	bash ./scripts/integration/xray-user-route-netns.sh embedded same
	bash ./scripts/integration/xray-user-route-netns.sh external separate
	bash ./scripts/integration/xray-user-route-netns.sh external same

integration-tcp-tun-netns:
	bash ./scripts/integration/raw-tcp-tun-netns.sh

integration-tcp-tun-ipv6-underlay-netns:
	UNDERLAY_FAMILY=ipv6 bash ./scripts/integration/raw-tcp-tun-netns.sh

integration-tcp-vkey-route-netns:
	bash ./scripts/integration/raw-tcp-vkey-route-netns.sh

integration-tcp-tls-vkey-route-netns:
	SECURITY=tls bash ./scripts/integration/raw-tcp-vkey-route-netns.sh

integration-tcp-tls-tun-netns:
	bash ./scripts/integration/raw-tcp-tls-tun-netns.sh

integration-udp-dtls-tun-netns:
	bash ./scripts/integration/raw-udp-dtls-tun-netns.sh

integration-address-guard-netns:
	bash ./scripts/integration/address-guard-netns.sh

integration-user-rate-limit-netns:
	bash ./scripts/integration/user-rate-limit-netns.sh

integration-user-rate-limit-xray-netns:
	TRANSPORT=xray bash ./scripts/integration/user-rate-limit-netns.sh

integration-user-rate-limit-xray-external-netns:
	mkdir -p ./build/lab
	CGO_ENABLED=0 go build -trimpath -o ./build/lab/xray-linux-amd64 github.com/xtls/xray-core/main
	TRANSPORT=xray-external XRAY_BIN="$$(pwd)/build/lab/xray-linux-amd64" bash ./scripts/integration/user-rate-limit-netns.sh

clean:
	$(MAKE) -C core/fastpath clean
	rm -rf bin build dist out coverage
