'use strict';
'require rpc';
'require ui';
'require view';

var callGetEnv = rpc.declare({object: 'olcrtc-bot', method: 'get_env', expect: {}});
var callSetEnv = rpc.declare({object: 'olcrtc-bot', method: 'set_env', params: ['env', 'restart']});

/* Fields edited on the Control Plane. OLCRTC_KEY is edited on the Data Plane
 * and is preserved by the rpcd merge, so it is intentionally NOT listed here. */
var controlFields = [
	{key: 'IS_CLIENT', section: 'role', type: 'role'},
	{key: 'VK_TOKEN', label: 'VK_TOKEN', section: 'bot', type: 'password'},
	{key: 'ALLOWED_USER_IDS', label: 'ALLOWED_USER_IDS', section: 'bot', type: 'text',
		hint: _('Comma-separated VK user IDs (empty = allow all)')},
	{key: 'SLEEP_ON_ERROR', label: 'SLEEP_ON_ERROR', section: 'advanced', type: 'text', suffix: _('sec')},
	{key: 'OLCRTC_EXE', label: 'OLCRTC_EXE', section: 'advanced', type: 'text'},
	{key: 'DEBUG', label: 'DEBUG', section: 'advanced', type: 'checkbox'},
	{key: 'DATA_DIR', label: 'DATA_DIR', section: 'advanced', type: 'text'},
	{key: 'LAST_PROVIDER_FILE', label: 'LAST_PROVIDER_FILE', section: 'advanced', type: 'text'},
	{key: 'STATUS_PORT', label: 'STATUS_PORT', section: 'advanced', type: 'text',
		hint: _('Local status endpoint port (loopback). Default 9091.')}
];

return view.extend({
	_env: {},

	load: function () {
		return Promise.all([
			L.require('view/olcrtc/statusbar'),
			callGetEnv()
		]);
	},

	render: function (data) {
		var statusbar = data[0];
		this._env = data[1] || {};

		var container = E('div', {'id': 'olcrtc-control'});
		statusbar.mount(container);
		container.appendChild(this._buildForm());
		return container;
	},

	_fieldValue: function (key) {
		var v = this._env[key];
		return (v === undefined || v === null) ? '' : v;
	},

	_buildForm: function () {
		var self = this;

		/* ── Global Mode (role) ─────────────────────────── */
		var isClient = this._fieldValue('IS_CLIENT') === 'true' || this._fieldValue('IS_CLIENT') === '1';
		var clientRadio = E('input', {
			'type': 'radio', 'name': 'role-select', 'id': 'env-IS_CLIENT-client', 'value': 'true'
		});
		var serverRadio = E('input', {
			'type': 'radio', 'name': 'role-select', 'id': 'env-IS_CLIENT-server', 'value': 'false'
		});
		if (isClient) clientRadio.checked = true; else serverRadio.checked = true;
		this._roleRadios = {true: clientRadio, false: serverRadio};

		var roleSection = E('div', {'class': 'cbi-section'}, [
			E('h3', {}, [_('Global Mode')]),
			E('div', {'style': 'margin:6px 0'}, [
				E('label', {'style': 'margin-right:20px'}, [
					clientRadio, ' \ud83d\udcfa ' + _('Client') + ' ',
					E('span', {'style': 'color:#888;font-size:0.85em'}, ['(cnc + tproxy)'])
				]),
				E('label', {}, [
					serverRadio, ' \ud83d\udce1 ' + _('Server') + ' ',
					E('span', {'style': 'color:#888;font-size:0.85em'}, ['(srv only)'])
				])
			]),
			E('p', {'class': 'cbi-section-descr'}, [
				'\u26a0 ' + _('Changing role requires a full service restart.')
			])
		]);

		/* ── VK Bot Configuration ───────────────────────── */
		var botRows = [];
		for (var i = 0; i < controlFields.length; i++) {
			var f = controlFields[i];
			if (f.section !== 'bot') continue;
			botRows.push(E('tr', {}, [
				E('td', {'width': '25%'}, [f.label]),
				E('td', {}, [this._buildField(f)])
			]));
		}

		var botSection = E('div', {'class': 'cbi-section'}, [].concat(
			[E('h3', {}, [_('VK Bot Configuration (/etc/dial-up.env)')])],
			botRows,
			[E('div', {'class': 'cbi-page-actions'}, [
				E('button', {
					'class': 'cbi-button cbi-button-apply important',
					'click': function () { self._doSave(true); }
				}, ['\ud83d\udcbe ' + _('Save & Restart Bot')])
			])]
		));

		/* ── Advanced (collapsible) ─────────────────────── */
		var advRows = [];
		for (var j = 0; j < controlFields.length; j++) {
			var af = controlFields[j];
			if (af.section !== 'advanced') continue;
			advRows.push(E('tr', {}, [
				E('td', {'width': '25%'}, [af.label]),
				E('td', {}, [this._buildField(af)])
			]));
		}

		var advanced = E('details', {'style': 'margin-top:4px'}, [
			E('summary', {'style': 'cursor:pointer;font-weight:bold'}, [_('Advanced Settings (Backoff & Paths)')]),
			E('table', {'class': 'table', 'width': '100%', 'style': 'margin-top:8px'}, advRows)
		]);
		botSection.appendChild(advanced);

		return E('div', {}, [roleSection, botSection]);
	},

	_buildField: function (f) {
		var val = this._fieldValue(f.key);
		var children = [];

		if (f.type === 'checkbox') {
			var cb = E('input', {'type': 'checkbox', 'id': 'env-' + f.key, 'class': 'cbi-input-checkbox'});
			cb.checked = (val === 'true' || val === '1');
			return cb;
		}

		if (f.type === 'password') {
			var pass = E('input', {
				'type': 'password', 'id': 'env-' + f.key, 'class': 'cbi-input-text',
				'style': 'width:70%', 'value': val
			});
			var toggle = E('button', {
				'class': 'cbi-button cbi-button-neutral', 'style': 'margin-left:4px',
				'click': function () { pass.type = (pass.type === 'password' ? 'text' : 'password'); }
			}, ['\ud83d\udc41']);
			return E('span', {}, [pass, ' ', toggle]);
		}

		/* text */
		var input = E('input', {
			'type': 'text', 'id': 'env-' + f.key, 'class': 'cbi-input-text',
			'style': 'width:60%', 'value': val
		});
		children.push(input);
		if (f.suffix) children.push(' ' + f.suffix);
		if (f.hint) children.push(E('span', {'style': 'color:#888;font-size:0.85em;margin-left:8px'}, [f.hint]));
		return E('span', {}, children);
	},

	_collectEnv: function () {
		var env = {};
		for (var i = 0; i < controlFields.length; i++) {
			var f = controlFields[i];
			if (f.type === 'role') {
				env[f.key] = this._roleRadios[true].checked ? 'true' : 'false';
				continue;
			}
			var el = document.getElementById('env-' + f.key);
			if (!el) continue;
			env[f.key] = (f.type === 'checkbox') ? (el.checked ? 'true' : 'false') : el.value;
		}
		return env;
	},

	_doSave: function (restart) {
		var env = this._collectEnv();
		callSetEnv(env, restart).then(function () {
			ui.addNotification(null, E('p', {}, [
				_('Environment saved') + (restart ? ', ' + _('service restarted') : '')
			]), 'info');
		}).catch(function (err) {
			ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
		});
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
