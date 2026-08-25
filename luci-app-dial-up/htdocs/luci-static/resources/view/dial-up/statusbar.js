'use strict';
'require rpc';
'require poll';
'require baseclass';

var callStatus = rpc.declare({
	object: 'dial-up-bot',
	method: 'get_status',
	expect: {}
});
var callGetSingbox = rpc.declare({
	object: 'dial-up-bot',
	method: 'get_singbox',
	expect: {}
});
var callGetEnv = rpc.declare({
	object: 'dial-up-bot',
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

	// Clients (server only): peer count parsed by the Go bot from olcrtc's
	// "Current peers count: N" lines. Absent/unknown on the client router.
	var clients;
	if (!isClient) {
		var pc = status.peers;
		if (typeof pc === 'number' || (pc != null && pc !== '')) {
			clients = { text: '' + pc, type: pc > 0 ? 'ok' : 'neutral' };
		} else {
			clients = { text: '—', type: 'neutral' };
		}
	}

	// Tunnel State — client uses RTT as authoritative; server uses process running
	var tunnelType, tunnelText;

	if (isClient && rttMs != null) {
		if (rttMs < 1000) {
			tunnelType = 'ok';
			tunnelText = _('Stable');
		} else {
			tunnelType = 'warning';
			tunnelText = _('Unstable');
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
	var routeText, routeType, routeIcon, directOn = false;

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
		directOn = singbox.direct_online === true || singbox.direct_online === 'true';

		if (route === 'proxy') {
			routeText = 'PROXY';
			routeType = 'ok'; // Green active state
		} else if (route === 'awg') {
			routeText = 'AWG';
			var awgOn = singbox.awg_online === true || singbox.awg_online === 'true';
			routeType = awgOn ? 'ok' : 'warning'; // Green when online, yellow while handshake pending
		} else if (route === 'direct') {
			routeText = 'DIRECT';
			routeType = directOn ? 'neutral' : 'bad'; // Red when direct internet is unavailable
		} else {
			routeText = '\u2014';
			routeType = 'bad'; // Red unknown state
		}
	}

	var routeInfo = { text: routeText, type: routeType, icon: routeIcon, label: isClient ? _('LAN Route') : _('Egress') };
	if (isClient) {
		routeInfo.direct = { ok: directOn, text: directOn ? _('Free') : _('Restricted') };
	}

	return {
		bot: { text: botText, type: botType },
		tunnel: { text: tunnelText, type: tunnelType },
		clients: clients,
		route: routeInfo
	};
}

function renderStatusStrip(status, singbox, env) {
	var s = describe(status, singbox, env);

	function makeCard(icon, label, info) {
		return E('div', { 'class': 'dial-up-metric-card' }, [
			E('div', { 'class': 'dial-up-icon-wrap' }, [icon]),
			E('div', { 'class': 'dial-up-info-wrap' }, [
				E('div', { 'class': 'dial-up-label' }, [label]),
				E('div', { 'class': 'dial-up-value-wrap' }, [
					E('span', { 'class': 'dial-up-dot ' + info.type }),
					E('span', { 'class': 'dial-up-value' }, [info.text])
				])
			])
		]);
	}

	function makeTunnelCard(info) {
		var rtt = (status.tunnel_rtt_ms == null) ? '\u2014' : (status.tunnel_rtt_ms + 'ms');
		var dns = status.dns_ping || '\u2014';

		return E('div', { 'class': 'dial-up-metric-card dial-up-tunnel-card' }, [
			E('div', { 'class': 'dial-up-icon-wrap' }, ['\ud83d\ude87']),
			E('div', { 'class': 'dial-up-info-wrap' }, [
				E('div', { 'class': 'dial-up-label' }, [_('Tunnel')]),
				E('div', { 'class': 'dial-up-value-wrap' }, [
					E('span', { 'class': 'dial-up-dot ' + info.type }),
					E('span', { 'class': 'dial-up-value' }, [info.text])
				])
			]),
			E('div', { 'class': 'dial-up-tunnel-extra' }, [
				E('div', { 'class': 'dial-up-extra-row' }, [
					E('span', { 'class': 'dial-up-extra-key' }, [_('RTT')]),
					' ' + rtt
				]),
				E('div', { 'class': 'dial-up-extra-row' }, [
					E('span', { 'class': 'dial-up-extra-key' }, [_('DNS')]),
					' ' + dns
				])
			])
		]);
	}

	function makeRouteCard(info) {
		var card = E('div', { 'class': 'dial-up-metric-card' }, [
			E('div', { 'class': 'dial-up-icon-wrap' }, [info.icon]),
			E('div', { 'class': 'dial-up-info-wrap' }, [
				E('div', { 'class': 'dial-up-label' }, [info.label]),
				E('div', { 'class': 'dial-up-value-wrap' }, [
					E('span', { 'class': 'dial-up-dot ' + info.type }),
					E('span', { 'class': 'dial-up-value' }, [info.text])
				])
			])
		]);
		if (info.direct) {
			card.appendChild(E('div', { 'class': 'dial-up-tunnel-extra dial-up-route-extra' }, [
				E('div', { 'class': 'dial-up-label' }, [_('Internet')]),
				E('div', { 'class': 'dial-up-value-wrap' }, [
					E('span', { 'class': 'dial-up-dot ' + (info.direct.ok ? 'ok' : 'bad') }),
					E('span', { 'class': 'dial-up-value dial-up-internet' }, [info.direct.text])
				])
			]));
		}
		return card;
	}

	var cards = [
		makeCard('\ud83e\udd16', _('Bot'), s.bot),
		makeTunnelCard(s.tunnel)
	];
	if (s.clients) {
		cards.push(makeCard('\ud83d\udc65', _('Clients'), s.clients));
	}
	cards.push(makeRouteCard(s.route));

	return E('div', {
		'class': 'dial-up-status-panel',
		'style': 'display: flex; flex-wrap: wrap; gap: 16px; margin-bottom: 24px;'
	}, cards);
}

return baseclass.extend({
	mount: function (container) {
		var self = this;

		// Inject modern CSS globally once per page load
		if (!document.getElementById('dial-up-modern-styles')) {
			var style = E('style', { 'id': 'dial-up-modern-styles' }, [
				'@keyframes dial-up-pulse { 0% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0.4); } 70% { box-shadow: 0 0 0 8px rgba(67, 160, 71, 0); } 100% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0); } }',
				'@keyframes dial-up-pulse-warning { 0% { box-shadow: 0 0 0 0 rgba(253, 216, 53, 0.5); } 70% { box-shadow: 0 0 0 8px rgba(253, 216, 53, 0); } 100% { box-shadow: 0 0 0 0 rgba(253, 216, 53, 0); } }',
				'.dial-up-metric-card { display: flex; align-items: center; gap: 14px; padding: 14px 20px; background: rgba(128,128,128,0.06); border: 1px solid rgba(128,128,128,0.12); border-radius: 12px; flex: 1; min-width: 220px; box-shadow: 0 2px 8px rgba(0,0,0,0.04); transition: transform 0.2s, background 0.2s; }',
				'.dial-up-metric-card:hover { background: rgba(128,128,128,0.09); transform: translateY(-1px); }',
				'.dial-up-icon-wrap { display: flex; align-items: center; justify-content: center; width: 46px; height: 46px; border-radius: 12px; font-size: 1.6em; background: rgba(128,128,128,0.08); border: 1px solid rgba(128,128,128,0.05); }',
				'.dial-up-info-wrap { display: flex; flex-direction: column; justify-content: center; }',
				'.dial-up-label { font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.5px; opacity: 0.6; font-weight: 700; margin-bottom: 4px; }',
				'.dial-up-value-wrap { display: flex; align-items: center; gap: 8px; }',
				'.dial-up-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }',
				'.dial-up-dot.ok { background: #43a047; animation: dial-up-pulse 2s infinite; }',
				'.dial-up-dot.bad { background: #e53935; }',
				'.dial-up-dot.neutral { background: #1e88e5; }',
				'.dial-up-dot.warning { background: #fdd835; animation: dial-up-pulse-warning 2s infinite; }',
				'.dial-up-value { font-size: 1.15em; font-weight: 600; }',
				'.dial-up-tunnel-card { min-width: 360px; }',
				'.dial-up-tunnel-extra { display: flex; flex-direction: column; justify-content: center; gap: 4px; padding-left: 14px; border-left: 1px solid rgba(128,128,128,0.22); }',
				'.dial-up-extra-row { font-weight: 600; white-space: nowrap; }',
				'.dial-up-extra-key { text-transform: uppercase; letter-spacing: 0.5px; }',
				'.dial-up-internet { text-transform: uppercase; }',
				'.dial-up-route-extra { gap: 0; align-self: stretch; }'
			].join('\n'));
			document.head.appendChild(style);
		}

		this.node = E('div', { 'id': 'dial-up-statusbar-mount' });
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
	}
});
