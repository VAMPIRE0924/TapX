.PHONY: all test go-test fastpath verify-local verify-release build-linux-amd64 build-openwrt-x86 package-openwrt-x86 install-linux integration-netns integration-device-apply-netns integration-bridge-apply-netns integration-mss-clamp-netns integration-dns-apply-netns integration-tun-netns integration-tun-vkey-netns integration-tap-netns integration-tcp-tun-netns integration-tcp-tls-tun-netns integration-udp-dtls-tun-netns integration-address-guard-netns clean env

CC ?= gcc
CFLAGS ?= -std=c11 -O3 -Wall -Wextra -Werror -fPIC

all: test

env:
	bash ./scripts/dev/check-env.sh

test: go-test fastpath

go-test:
	go test ./...

fastpath:
	$(MAKE) -C core/fastpath test

verify-local:
	go run ./scripts/verify

verify-release:
	go run ./scripts/verify -require-openwrt-ipk

build-linux-amd64:
	bash ./scripts/build/linux-amd64.sh

build-openwrt-x86:
	bash ./scripts/build/openwrt-x86-64.sh

package-openwrt-x86: build-openwrt-x86
	bash ./scripts/build/openwrt-x86-64-ipk.sh

install-linux: build-linux-amd64
	bash ./scripts/install/linux-install.sh

integration-netns: integration-device-apply-netns integration-bridge-apply-netns integration-mss-clamp-netns integration-dns-apply-netns integration-tun-netns integration-tun-vkey-netns integration-tap-netns integration-tcp-tun-netns integration-tcp-tls-tun-netns integration-udp-dtls-tun-netns integration-address-guard-netns

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

integration-tun-vkey-netns:
	bash ./scripts/integration/raw-udp-tun-vkey-netns.sh

integration-tap-netns:
	bash ./scripts/integration/raw-udp-tap-netns.sh

integration-tcp-tun-netns:
	bash ./scripts/integration/raw-tcp-tun-netns.sh

integration-tcp-tls-tun-netns:
	bash ./scripts/integration/raw-tcp-tls-tun-netns.sh

integration-udp-dtls-tun-netns:
	bash ./scripts/integration/raw-udp-dtls-tun-netns.sh

integration-address-guard-netns:
	bash ./scripts/integration/address-guard-netns.sh

clean:
	$(MAKE) -C core/fastpath clean
	rm -rf bin build dist out coverage
