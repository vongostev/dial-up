'use strict';
'require rpc';
'require poll';
'require baseclass';

var callStatus = rpc.declare({
	object: 'olcrtc-bot',
	method: 'get_status',
	expect: {}
});
var callGetSingbox = rpc.declare({
	object: 'olcrtc-bot',
	method: 'get_singbox',
	expect: {}
});
var callGetEnv = rpc.declare({
	object: 'olcrtc-bot',
	method: 'get_env',
	expect: {}
});

function describe(status, singbox, env) {
	status = status || {};
	singbox = singbox || {};
	env = env || {};
	var isClient = status.is_client === true;
	var rttMs = status.tunnel_rtt_ms;

	// Bot State
	var botOk = !!status.vk_alive;
	var botType = botOk ? 'ok' : 'bad';
	var botText = botOk ? _('Online') : _('Offline');

	// Tunnel State — client uses RTT as authoritative; server uses process running
	var tunnelType, tunnelText;

	if (isClient && rttMs != null) {
		if (rttMs < 1000) {
			tunnelType = 'ok';
			tunnelText = _('Stable') + ' (RTT ' + rttMs + 'ms)';
		} else {
			tunnelType = 'warning';
			tunnelText = _('Unstable') + ' (RTT ' + rttMs + 'ms)';
		}
	} else if (isClient && !!status.running) {
		tunnelType = 'warning';
		tunnelText = _('Degraded');
	} else if (isClient) {
		tunnelType = 'bad';
		tunnelText = _('Down');
	} else {
		var tunnelOk = !!status.running;

		tunnelType = tunnelOk ? 'ok' : 'bad';
		tunnelText = tunnelOk ? _('Up') : _('Down');
	}

	// 3rd card: client → LAN Route; server → EGRESS
	var routeText, routeType, routeIcon;

	if (!isClient) {
		var addr = env.SOCKS_PROXY_ADDR || '';
		if (addr) {
			routeText = addr + ':' + (env.SOCKS_PROXY_PORT || '');
			routeType = 'ok';
			routeIcon = '\ud83c\udf10';
		} else {
			routeText = _('DIRECT');
			routeType = 'neutral';
			routeIcon = '\ud83c\udf10';
		}
	} else {
		routeIcon = '\ud83d\udea6';
		// Network Route State (Supports both 'now' from Clash API and 'route' fallback)
		var route = singbox.now || singbox.route || 'unknown';

		if (route === 'proxy') {
			routeText = 'PROXY';
			routeType = 'ok'; // Green active state
		} else if (route === 'direct') {
			routeText = 'DIRECT';
			routeType = 'neutral'; // Blue informational state
		} else {
			routeText = '\u2014';
			routeType = 'bad'; // Red unknown state
		}
	}

	return {
		bot: { text: botText, type: botType },
		tunnel: { text: tunnelText, type: tunnelType },
		route: { text: routeText, type: routeType, icon: routeIcon, label: isClient ? _('LAN Route') : _('Egress') }
	};
}

function renderStatusStrip(status, singbox, env) {
	var s = describe(status, singbox, env);

	function makeCard(icon, label, info) {
		return E('div', { 'class': 'olcrtc-metric-card' }, [
			E('div', { 'class': 'olcrtc-icon-wrap' }, [icon]),
			E('div', { 'class': 'olcrtc-info-wrap' }, [
				E('div', { 'class': 'olcrtc-label' }, [label]),
				E('div', { 'class': 'olcrtc-value-wrap' }, [
					E('span', { 'class': 'olcrtc-dot ' + info.type }),
					E('span', { 'class': 'olcrtc-value' }, [info.text])
				])
			])
		]);
	}

	return E('div', {
		'class': 'olcrtc-status-panel',
		'style': 'display: flex; flex-wrap: wrap; gap: 16px; margin-bottom: 24px;'
	}, [
		makeCard('\ud83e\udd16', _('Bot'), s.bot),
		makeCard('\ud83d\ude87', _('Tunnel'), s.tunnel),
		makeCard(s.route.icon, s.route.label, s.route)
	]);
}

return baseclass.extend({
	mount: function (container) {
		var self = this;

		// Inject modern CSS globally once per page load
		if (!document.getElementById('olcrtc-modern-styles')) {
			var style = E('style', { 'id': 'olcrtc-modern-styles' }, [
				'@keyframes olcrtc-pulse { 0% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0.4); } 70% { box-shadow: 0 0 0 8px rgba(67, 160, 71, 0); } 100% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0); } }',
				'@keyframes olcrtc-pulse-warning { 0% { box-shadow: 0 0 0 0 rgba(253, 216, 53, 0.5); } 70% { box-shadow: 0 0 0 8px rgba(253, 216, 53, 0); } 100% { box-shadow: 0 0 0 0 rgba(253, 216, 53, 0); } }',
				'.olcrtc-metric-card { display: flex; align-items: center; gap: 14px; padding: 14px 20px; background: rgba(128,128,128,0.06); border: 1px solid rgba(128,128,128,0.12); border-radius: 12px; flex: 1; min-width: 220px; box-shadow: 0 2px 8px rgba(0,0,0,0.04); transition: transform 0.2s, background 0.2s; }',
				'.olcrtc-metric-card:hover { background: rgba(128,128,128,0.09); transform: translateY(-1px); }',
				'.olcrtc-icon-wrap { display: flex; align-items: center; justify-content: center; width: 46px; height: 46px; border-radius: 12px; font-size: 1.6em; background: rgba(128,128,128,0.08); border: 1px solid rgba(128,128,128,0.05); }',
				'.olcrtc-info-wrap { display: flex; flex-direction: column; justify-content: center; }',
				'.olcrtc-label { font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.5px; opacity: 0.6; font-weight: 700; margin-bottom: 4px; }',
				'.olcrtc-value-wrap { display: flex; align-items: center; gap: 8px; }',
				'.olcrtc-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }',
				'.olcrtc-dot.ok { background: #43a047; animation: olcrtc-pulse 2s infinite; }',
				'.olcrtc-dot.bad { background: #e53935; }',
				'.olcrtc-dot.neutral { background: #1e88e5; }',
				'.olcrtc-dot.warning { background: #fdd835; animation: olcrtc-pulse-warning 2s infinite; }',
				'.olcrtc-value { font-size: 1.15em; font-weight: 600; }'
			].join('\n'));
			document.head.appendChild(style);
		}

		this.node = E('div', { 'id': 'olcrtc-statusbar-mount' });
		container.appendChild(this.node);
		this.refresh();

		if (this.pollFn) poll.remove(this.pollFn);
		this.pollFn = function () { return self.refresh(); };
		poll.add(this.pollFn, 5);

		return this.node;
	},

	refresh: function () {
		var self = this;
		var calls = [callStatus(), callGetSingbox(), callGetEnv()];

		return Promise.all(calls).then(function (res) {
			if (self.node && self.node.parentNode) {
				var fresh = renderStatusStrip(res[0], res[1], res[2]);
				self.node.parentNode.replaceChild(fresh, self.node);
				self.node = fresh;
			}
		}).catch(function () {});
	},

	renderStatusStrip: function (status, singbox, env) {
		return renderStatusStrip(status, singbox, env);
	}
});