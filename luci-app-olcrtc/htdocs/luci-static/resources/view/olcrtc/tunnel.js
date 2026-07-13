'use strict';
'require rpc';
'require ui';
'require view';
'require poll';

var callStatus = rpc.declare({object: 'olcrtc-bot', method: 'get_status', expect: {}});
var callSetProvider = rpc.declare({object: 'olcrtc-bot', method: 'set_provider', params: ['url']});
var callClearProvider = rpc.declare({object: 'olcrtc-bot', method: 'clear_provider'});
var callServiceAction = rpc.declare({object: 'olcrtc-bot', method: 'service_action', params: ['action']});
var callGetEnv = rpc.declare({object: 'olcrtc-bot', method: 'get_env', expect: {}});
var callSetEnv = rpc.declare({object: 'olcrtc-bot', method: 'set_env', params: ['env', 'restart']});
var callGenerateKey = rpc.declare({object: 'olcrtc-bot', method: 'generate_key', expect: {}});

var MAX_CRASH_FAILURES = 5;

function fmtUptime(secs) {
	if (!secs || secs < 0) return '\u2014';
	var d = Math.floor(secs / 86400),
		h = Math.floor((secs % 86400) / 3600),
		m = Math.floor((secs % 3600) / 60),
		s = secs % 60;
	var parts = [];
	if (d > 0) parts.push(d + 'd');
	if (h > 0) parts.push(h + 'h');
	if (m > 0) parts.push(m + 'm');
	parts.push(s + 's');
	return parts.join(' ');
}

function parseProvider(prov) {
	var kind = '', id = '';
	if (prov) {
		if (Array.isArray(prov) && prov.length >= 2) { kind = prov[0]; id = prov[1]; }
		else if (prov.Kind) { kind = prov.Kind; id = prov.RoomID || prov.room_id || ''; }
		else if (prov.kind) { kind = prov.kind; id = prov.room_id || ''; }
	}
	return {kind: kind, id: id};
}

return view.extend({
	_pollFn: null,
	_metricsContainer: null,

	load: function () {
		return Promise.all([
			L.require('view/olcrtc/statusbar'),
			callStatus(),
			callGetEnv()
		]);
	},

	render: function (data) {
		var statusbar = data[0];
		var status = data[1] || {};
		this._env = data[2] || {};

		var container = E('div', {'id': 'olcrtc-data'});
		statusbar.mount(container);
		container.appendChild(this._buildConnection());
		this._metricsContainer = E('div', {'id': 'olcrtc-data-metrics'});
		container.appendChild(this._metricsContainer);
		container.appendChild(this._buildCrypto());
		this._renderMetrics(status);
		this._setupPoll();
		return container;
	},

	_buildConnection: function () {
		var self = this;
		var input = E('input', {
			'type': 'text', 'id': 'olcrtc-provider-url', 'class': 'cbi-input-text',
			'style': 'width:100%',
			'placeholder': 'https://stream.wb.ru/room/uuid or https://telemost.yandex.ru/j/id...'
		});

		return E('div', {'class': 'cbi-section'}, [
			E('h3', {}, [_('Connection Setup')]),
			E('table', {'class': 'table', 'width': '100%'}, [
				E('tr', {}, [E('td', {'width': '15%'}, [_('Provider URL')]), E('td', {}, [input])])
			]),
			E('div', {'class': 'cbi-page-actions'}, [
				E('button', {
					'class': 'cbi-button cbi-button-apply important',
					'click': function () {
						var url = (document.getElementById('olcrtc-provider-url').value || '').trim();
						if (!url) {
							ui.addNotification(null, E('p', {}, [_('Please enter a URL')]), 'error');
							return;
						}
						callSetProvider({url: url}).then(function (res) {
							if (res.error) {
								ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + res.error]), 'error');
							} else {
								ui.addNotification(null, E('p', {}, [_('Provider set successfully')]), 'info');
								document.getElementById('olcrtc-provider-url').value = '';
								setTimeout(function () { self._refresh(); }, 3000);
							}
						}).catch(function (err) {
							ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
						});
					}
				}, ['\u25b6 ' + _('Connect / Update')]),
				' ',
				E('button', {
					'class': 'cbi-button cbi-button-remove',
					'click': function () {
						if (!confirm(_('Stop tunnel and clear provider?'))) return;
						callClearProvider().then(function () {
							ui.addNotification(null, E('p', {}, [_('Provider cleared, service stopped')]), 'info');
							setTimeout(function () { self._refresh(); }, 3000);
						}).catch(function (err) {
							ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
						});
					}
				}, ['\u23f9 ' + _('Stop & Clear')]),
				' ',
				E('button', {
					'class': 'cbi-button cbi-button-reload',
					'click': function () {
						callServiceAction({action: 'restart'}).then(function () {
							ui.addNotification(null, E('p', {}, [_('Service') + ' restart ' + _('executed')]), 'info');
							setTimeout(function () { self._refresh(); }, 3000);
						}).catch(function (err) {
							ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
						});
					}
				}, ['\ud83d\udcbe ' + _('Save & Restart OlcRTC')])
			])
		]);
	},

	_renderMetrics: function (status) {
		var container = this._metricsContainer;
		while (container.firstChild) container.removeChild(container.firstChild);

	var running = !!status.running;
	var isClient = status.is_client === true;
	var rttMs = status.tunnel_rtt_ms;
	var prov = parseProvider(status.provider);
	var transport = status.transport || 'vp8channel';
	var hasError = status.last_error && status.last_error !== '';

	var stableNote = '';
	var olcrtcStatusText, olcrtcStatusEmoji;
	if (isClient && rttMs != null) {
		olcrtcStatusEmoji = '\ud83d\udfe2';
		olcrtcStatusText = _('Running');
		stableNote = (running && status.uptime_seconds && status.uptime_seconds > 30)
			? ' (' + _('Stable > 30s') + ')' : '';
	} else if (isClient && running) {
		olcrtcStatusEmoji = '\ud83d\udfe0';
		olcrtcStatusText = _('Degraded');
	} else if (running) {
		olcrtcStatusEmoji = '\ud83d\udfe2';
		olcrtcStatusText = _('Running');
		stableNote = (running && status.uptime_seconds && status.uptime_seconds > 30)
			? ' (' + _('Stable > 30s') + ')' : '';
	} else {
		olcrtcStatusEmoji = '\ud83d\udd34';
		olcrtcStatusText = _('Stopped');
	}

	var rows = [
			E('tr', {}, [E('td', {'width': '30%'}, [_('OlcRTC Status')]),
				E('td', {}, [olcrtcStatusEmoji, ' ', olcrtcStatusText + stableNote])]),
			E('tr', {}, [E('td', {}, [_('Current Provider')]),
				E('td', {}, [prov.kind ? prov.kind + ' \u00b7 ' + prov.id : '\u2014'])]),
			E('tr', {}, [E('td', {}, [_('Active Carrier')]),
				E('td', {}, [transport])])
		];

		container.appendChild(E('div', {'class': 'cbi-section'}, [
			E('h3', {}, [_('Tunnel Metrics & Health (Live)')]),
			E('table', {'class': 'table', 'width': '100%'}, rows)
		]));

		/* ── Metrics ────────────────────────────────────── */
		var metricRows = [
			E('tr', {}, [E('td', {'width': '30%'}, [_('Uptime')]), E('td', {}, [fmtUptime(status.uptime_seconds)])]),
			E('tr', {}, [E('td', {}, [_('DNS Ping (9.9.9.9)')]), E('td', {}, [status.dns_ping || '\u2014'])]),
			E('tr', {}, [E('td', {}, [_('Backoff / Fails')]),
				E('td', {}, [(status.failures || 0) + ' / ' + (status.crash_failures || 0) + ' ' +
					_('(max %d)').replace('%d', MAX_CRASH_FAILURES)])])
		];

		container.appendChild(E('div', {'class': 'cbi-section'}, [
			E('h4', {}, [_('Metrics')]),
			E('table', {'class': 'table', 'width': '100%'}, metricRows)
		]));

		/* ── Diagnostics ────────────────────────────────── */
		var lastExit = (status.last_exit_code === null || status.last_exit_code === undefined) ? '\u2014' : ('' + status.last_exit_code);
		container.appendChild(E('div', {'class': 'cbi-section'}, [
			E('h4', {}, [_('Diagnostics')]),
			E('table', {'class': 'table', 'width': '100%'}, [
				E('tr', {}, [E('td', {'width': '30%'}, [_('Last Exit Code')]), E('td', {}, [lastExit])]),
				E('tr', {}, [E('td', {}, [_('Last Error')]),
					hasError
						? E('td', {'style': 'color:#f44336'}, [status.last_error])
						: E('td', {}, [_('none')])])
			]),
			E('p', {'class': 'cbi-section-descr'}, [
				'\u24d8 ' + _('If the tunnel crashes, the exact error captured from the subprocess pipe is shown here.')
			])
		]));
	},

	_buildCrypto: function () {
		var val = (this._env && this._env.OLCRTC_KEY) ? this._env.OLCRTC_KEY : '';
		var pass = E('input', {
			'type': 'password', 'id': 'env-OLCRTC_KEY', 'class': 'cbi-input-text',
			'style': 'width:60%', 'value': val
		});
		var toggle = E('button', {
			'class': 'cbi-button cbi-button-neutral', 'style': 'margin-left:4px',
			'click': function () { pass.type = (pass.type === 'password' ? 'text' : 'password'); }
		}, ['\ud83d\udc41']);

		return E('div', {'class': 'cbi-section'}, [
			E('h3', {}, [_('Cryptography')]),
			E('table', {'class': 'table', 'width': '100%'}, [
				E('tr', {}, [E('td', {'width': '25%'}, ['OLCRTC_KEY']), E('td', {}, [pass, ' ', toggle])])
			]),
			E('div', {'class': 'cbi-page-actions'}, [
				E('button', {
					'class': 'cbi-button cbi-button-action',
					'click': function () {
						callGenerateKey().then(function (res) {
							if (res.key) {
								pass.value = res.key;
								pass.type = 'text';
							}
						}).catch(function (err) {
							ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
						});
					}
				}, ['\ud83d\udd04 ' + _('Generate New Key')]),
				' ',
				E('button', {
					'class': 'cbi-button cbi-button-apply important',
					'click': function () {
						callSetEnv({OLCRTC_KEY: pass.value}, true).then(function () {
							ui.addNotification(null, E('p', {}, [_('OLCRTC_KEY saved, service restarted')]), 'info');
						}).catch(function (err) {
							ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
						});
					}
				}, ['\ud83d\udcbe ' + _('Save Key & Restart')])
			]),
			E('p', {'class': 'cbi-section-descr'}, [
				'\u26a0 ' + _('Saving the key restarts dial-up.')
			])
		]);
	},

	_setupPoll: function () {
		var self = this;
		if (this._pollFn) poll.remove(this._pollFn);
		this._pollFn = function () {
			return callStatus().then(function (status) {
				if (self._metricsContainer) self._renderMetrics(status);
			}).catch(function () {});
		};
		poll.add(this._pollFn, 5);
	},

	_refresh: function () {
		var self = this;
		callStatus().then(function (status) {
			if (self._metricsContainer) self._renderMetrics(status);
		}).catch(function () {});
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
