#
# Copyright (C) 2026 OlcRTC Project
#
# This is free software, licensed under the GNU General Public License v2.
# See /LICENSE for more information.
#

include $(TOPDIR)/rules.mk

LUCI_TITLE:=LuCI Support for olcRTC VK Bot
LUCI_DEPENDS:=+rpcd +luci-mod-rpc
LUCI_PKGARCH:=all
LUCI_DESCRIPTION:=Web UI for monitoring and configuring dial-up on OpenWrt, organized into four semantic planes (Control / Data / Network / Unified Logs) with an always-visible global status strip backed by the bot's authoritative status endpoint.

PKG_NAME:=luci-app-olcrtc
PKG_VERSION:=2.0.0
PKG_RELEASE:=1

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt buildroot expects this
$(eval $(call BuildPackage,luci-app-olcrtc))