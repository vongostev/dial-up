# === Positional arguments (via make goals) ===
#
# Usage:
#   make deploy [client|server] [user@host]   VK_TOKEN=... OLCRTC_KEY=... [ALLOWED_USER_IDS=...] [SSH_PORT=...]
#   make remove  [user@host]
#
# Defaults: type=client, host=root@192.168.1.1, ssh port=22
#
KNOWN_GOALS := deploy remove build test lint fmt clean client server
_TYPE := $(filter client server,$(MAKECMDGOALS))
_HOST := $(filter-out $(KNOWN_GOALS),$(MAKECMDGOALS))

ROUTER_TYPE := $(if $(_TYPE),$(_TYPE),client)
ROUTER_HOST := $(if $(_HOST),$(_HOST),root@192.168.1.1)
IS_CLIENT   := $(if $(filter server,$(ROUTER_TYPE)),false,true)

VK_TOKEN         ?=
OLCRTC_KEY       ?=
ALLOWED_USER_IDS ?=
SOCKS_PROXY_ADDR ?=
SOCKS_PROXY_PORT ?=
SOCKS_PROXY_USER ?=
SOCKS_PROXY_PASS ?=
NET_DNS          ?= 127.0.0.1:53
SSH_PORT         ?= 22

.PHONY: build deploy remove test lint fmt clean client server

build:
	mage BuildArm64

deploy: build
	VK_TOKEN="$(VK_TOKEN)" OLCRTC_KEY="$(OLCRTC_KEY)" ALLOWED_USER_IDS="$(ALLOWED_USER_IDS)" SOCKS_PROXY_ADDR="$(SOCKS_PROXY_ADDR)" SOCKS_PROXY_PORT="$(SOCKS_PROXY_PORT)" SOCKS_PROXY_USER="$(SOCKS_PROXY_USER)" SOCKS_PROXY_PASS="$(SOCKS_PROXY_PASS)" NET_DNS="$(NET_DNS)" SSH_PORT="$(SSH_PORT)" ./deploy.sh "$(ROUTER_TYPE)" "$(ROUTER_HOST)"

remove:
	./remove.sh "$(ROUTER_HOST)"

test:
	mage Test

lint:
	golangci-lint run ./...
	gofmt -l -e . | grep -v '.kilo\|.kilocode' || true

fmt:
	gofmt -l -e . | grep -v '.kilo\|.kilocode' | xargs -r gofmt -w

clean:
	rm -rf bin/

# --- dummy targets to swallow positional make arguments ---
# `client`/`server` carry the type; the catch-all `%:` absorbs the
# arbitrary `user@host` goal. Without these, make would error with
# "No rule to make target".
client server:
	@:

%:
	@:
