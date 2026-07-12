'use strict';
'require rpc';
'require ui';
'require view';
'require poll';

var callGetLogs = rpc.declare({
	object: 'olcrtc-bot',
	method: 'get_logs',
	params: ['source', 'lines', 'filter'],
	expect: {}
});

/* Source → logread filter + display tag. 'all' merges the three in parallel. */
var SOURCES = {
	all: {filter: null, tag: null},
	bot: {filter: 'dial-up', tag: '[BOT]'},
	core: {filter: 'olcrtc', tag: '[CORE]'},
	singbox: {filter: 'sing-box', tag: '[S-BOX]'}
};

var MONTHS = {
	Jan: 1, Feb: 2, Mar: 3, Apr: 4, May: 5, Jun: 6,
	Jul: 7, Aug: 8, Sep: 9, Oct: 10, Nov: 11, Dec: 12
};

/* Parse a leading syslog timestamp "Mon DD HH:MM:SS" into a sortable number.
 * Returns null when the line has no parseable leading timestamp. */
function parseLogTs(line) {
	var m = line.match(/^([A-Z][a-z]{2})\s+(\d{1,2})\s+(\d{2}):(\d{2}):(\d{2})/);
	if (!m) return null;
	var mon = MONTHS[m[1]];
	if (!mon) return null;
	var day = parseInt(m[2], 10);
	var hh = parseInt(m[3], 10);
	var mm = parseInt(m[4], 10);
	var ss = parseInt(m[5], 10);
	return mon * 1000000 + day * 10000 + hh * 3600 + mm * 60 + ss;
}

/* Merge tagged lines from multiple sources, sorting by timestamp when parseable. */
function mergeTagged(buckets) {
	var entries = [];
	var seq = 0;
	for (var i = 0; i < buckets.length; i++) {
		var b = buckets[i];
		var lines = (b && b.lines) ? b.lines : [];
		for (var j = 0; j < lines.length; j++) {
			var raw = lines[j];
			var key = parseLogTs(raw);
			entries.push({
				key: (key === null) ? Number.MAX_SAFE_INTEGER - 1000000 + seq : key,
				seq: seq,
				text: b.tag + ' ' + raw
			});
			seq++;
		}
	}
	entries.sort(function (a, c) {
		if (a.key !== c.key) return a.key - c.key;
		return a.seq - c.seq;
	});
	return entries.map(function (e) { return e.text; });
}

return view.extend({
	_currentSource: 'all',
	_currentLines: 200,
	_currentFilter: '',
	_currentAutoRefresh: 5,
	_pollFn: null,
	_lastMerged: [],

	load: function () {
		return Promise.all([
			L.require('view/olcrtc/statusbar'),
			this._fetch()
		]);
	},

	render: function (data) {
		var statusbar = data[0];
		var lines = data[1] || [];

		var container = E('div', {'id': 'olcrtc-logs'});
		statusbar.mount(container);
		container.appendChild(this._buildControls());

		var logOutput = E('pre', {
			'id': 'log-output',
			'style': 'background:#1a1a2e;color:#e0e0e0;padding:10px;border-radius:4px;overflow:auto;max-height:600px;font-family:monospace;font-size:12px;white-space:pre-wrap;word-break:break-all'
		}, [lines.join('\n')]);
		container.appendChild(logOutput);
		setTimeout(function () { logOutput.scrollTop = logOutput.scrollHeight; }, 50);

		this._setupPoll();
		return container;
	},

	_buildControls: function () {
		var self = this;

		var sourceDefs = [
			{value: 'all', label: _('All Services')},
			{value: 'bot', label: _('Bot Only')},
			{value: 'core', label: _('OlcRTC')},
			{value: 'singbox', label: _('Sing-box')}
		];
		var radios = sourceDefs.map(function (src) {
			var r = E('input', {
				'type': 'radio', 'name': 'log-source', 'value': src.value,
				'change': function () { self._currentSource = src.value; self._doFetch(); }
			});
			if (src.value === self._currentSource) r.checked = true;
			return E('label', {'style': 'margin-right:15px'}, [r, ' ' + src.label]);
		});

		var linesSelect = E('select', {
			'change': function () { self._currentLines = parseInt(this.value) || 200; }
		}, [
			E('option', {'value': '50'}, ['50']),
			E('option', {'value': '100'}, ['100']),
			E('option', {'value': '200', 'selected': 'selected'}, ['200']),
			E('option', {'value': '500'}, ['500']),
			E('option', {'value': '1000'}, ['1000'])
		]);

		var filterInput = E('input', {
			'type': 'text', 'id': 'log-filter', 'class': 'cbi-input-text', 'style': 'width:200px',
			'keyup': function () { self._currentFilter = this.value; self._applyFilter(); }
		});

		var autoRefreshSelect = E('select', {
			'change': function () { self._currentAutoRefresh = parseInt(this.value) || 0; self._setupPoll(); }
		}, [
			E('option', {'value': '0'}, [_('Off')]),
			E('option', {'value': '5', 'selected': 'selected'}, ['5s']),
			E('option', {'value': '10'}, ['10s']),
			E('option', {'value': '30'}, ['30s'])
		]);

		return E('div', {'class': 'cbi-section'}, [
			E('div', {'style': 'margin-bottom:10px'}, [].concat([E('strong', {}, [_('Source') + ':']), ' '], radios)),
			E('div', {'style': 'margin-bottom:10px;display:flex;gap:10px;align-items:center;flex-wrap:wrap'}, [
				E('span', {}, [_('Lines') + ':']), linesSelect,
				E('span', {}, [_('Search') + ':']), filterInput,
				E('span', {}, [_('Auto-refresh') + ':']), autoRefreshSelect
			]),
			E('div', {'class': 'cbi-page-actions'}, [
				E('button', {
					'class': 'cbi-button cbi-button-reload', 'click': function () { self._doFetch(); }
				}, ['\ud83d\udd04 ' + _('Refresh')]),
				' ',
				E('button', {
					'class': 'cbi-button cbi-button-reset', 'click': function () { self._clearView(); }
				}, ['\ud83d\uddd1 ' + _('Clear Logs')]),
				' ',
				E('button', {
					'class': 'cbi-button cbi-button-download', 'click': function () { self._doDownload(); }
				}, ['\ud83d\udce5 ' + _('Download')])
			])
		]);
	},

	/* Fetch (and merge for 'all'); returns a Promise resolving to a string[] of tagged lines. */
	_fetch: function () {
		var self = this;
		var src = this._currentSource;
		var lines = this._currentLines;
		var filter = this._currentFilter;

		if (src === 'all') {
			var defs = [
				callGetLogs({source: 'dial-up', lines: lines, filter: ''}).then(function (d) { return {lines: (d && d.lines) || [], tag: '[BOT]'}; }).catch(function () { return {lines: [], tag: '[BOT]'}; }),
				callGetLogs({source: 'olcrtc', lines: lines, filter: ''}).then(function (d) { return {lines: (d && d.lines) || [], tag: '[CORE]'}; }).catch(function () { return {lines: [], tag: '[CORE]'}; }),
				callGetLogs({source: 'sing-box', lines: lines, filter: ''}).then(function (d) { return {lines: (d && d.lines) || [], tag: '[S-BOX]'}; }).catch(function () { return {lines: [], tag: '[S-BOX]'}; })
			];
			return Promise.all(defs).then(function (buckets) {
				var merged = mergeTagged(buckets);
				self._lastMerged = merged;
				return self._applyFilterTo(merged);
			});
		}

		var sdef = SOURCES[src];
		return callGetLogs({source: sdef.filter, lines: lines, filter: ''}).then(function (d) {
			var ls = (d && d.lines) || [];
			var tagged = ls.map(function (l) { return sdef.tag + ' ' + l; });
			self._lastMerged = tagged;
			return self._applyFilterTo(tagged);
		}).catch(function () { return []; });
	},

	_applyFilterTo: function (list) {
		var f = (this._currentFilter || '').toLowerCase();
		if (!f) return list;
		return list.filter(function (l) { return l.toLowerCase().indexOf(f) >= 0; });
	},

	_applyFilter: function () {
		var el = document.getElementById('log-output');
		if (el) el.textContent = this._applyFilterTo(this._lastMerged).join('\n');
	},

	_doFetch: function () {
		var self = this;
		this._fetch().then(function (lines) {
			var el = document.getElementById('log-output');
			if (el) {
				el.textContent = lines.join('\n');
				el.scrollTop = el.scrollHeight;
			}
		}).catch(function (err) {
			var el = document.getElementById('log-output');
			if (el) el.textContent = _('Error fetching logs:') + ' ' + (err.message || err);
		});
	},

	_clearView: function () {
		this._lastMerged = [];
		var el = document.getElementById('log-output');
		if (el) el.textContent = '';
		ui.addNotification(null, E('p', {}, [_('Log view cleared (logs on the server are unchanged).')]), 'info');
	},

	_setupPoll: function () {
		if (this._pollFn) poll.remove(this._pollFn);
		if (this._currentAutoRefresh <= 0) return;
		var self = this;
		this._pollFn = function () { return self._doFetch(); };
		poll.add(this._pollFn, this._currentAutoRefresh);
	},

	_doDownload: function () {
		var el = document.getElementById('log-output');
		if (!el) return;
		var blob = new Blob([el.textContent || ''], {type: 'text/plain'});
		var a = document.createElement('a');
		a.href = URL.createObjectURL(blob);
		a.download = 'olcrtc-' + (this._currentSource || 'logs') + '.txt';
		a.click();
		URL.revokeObjectURL(a.href);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
