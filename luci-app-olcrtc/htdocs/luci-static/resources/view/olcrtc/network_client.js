'use strict';
'require rpc';
'require ui';
'require poll';
'require baseclass';

var callStatus = rpc.declare({object: 'olcrtc-bot', method: 'get_status', expect: {}});
var callGetSingbox = rpc.declare({object: 'olcrtc-bot', method: 'get_singbox', expect: {}});
var callSetRoute = rpc.declare({object: 'olcrtc-bot', method: 'set_route', params: ['mode']});
var callGetFirewall = rpc.declare({object: 'olcrtc-bot', method: 'get_firewall', expect: {}});
var callGetWhitelist = rpc.declare({object: 'olcrtc-bot', method: 'get_whitelist', expect: {}});
var callSetWhitelist = rpc.declare({object: 'olcrtc-bot', method: 'set_whitelist', params: ['domains']});

function statusEmoji(ok) {
    return ok ? '\ud83d\udfe2' : '\ud83d\udd34';
}

return baseclass.extend({
    _wlDomains: [],
    _route: 'unknown',
    _topologySection: null,
    _pollFn: null,

    load: function () {
        return Promise.all([
            callStatus(),
            callGetSingbox(),
            callGetWhitelist(),
            callGetFirewall()
        ]);
    },

    render: function (container, data) {
        var status = data[0] || {};
        var singbox = data[1] || {};
        var whitelist = data[2] || {};
        var firewall = data[3] || {};

        this._route = singbox.now || singbox.route || 'unknown';

        this._topologySection = this._buildTopology(status, singbox);
        container.appendChild(this._topologySection);
        container.appendChild(this._buildWhitelist(whitelist));
        container.appendChild(this._buildRouteOverride());
        container.appendChild(this._buildDiagnostics(firewall));
        this._setupPoll();
    },

    /* -- Live Traffic Flow topology (Modern UI) ---------- */
    _buildTopology: function (status, singbox) {
        var activeRoute = singbox.now || singbox.route || 'unknown';

        if (!document.getElementById('olcrtc-topo-styles')) {
            var styleNode = E('style', {'id': 'olcrtc-topo-styles'}, [
                '.olcrtc-topo-container { display: flex; flex-direction: column; align-items: center; padding: 10px 0 20px 0; }',
                '.olcrtc-topo-card { background: rgba(128,128,128,0.06); border: 2px solid rgba(128,128,128,0.12); border-radius: 16px; padding: 16px 20px; min-width: 220px; display: flex; flex-direction: column; align-items: center; transition: all 0.3s ease; box-shadow: 0 4px 12px rgba(0,0,0,0.02); }',
                '.olcrtc-topo-card.active { border-color: #43a047; background: rgba(67, 160, 71, 0.08); }',
                '.olcrtc-topo-card.active-pulse { animation: olcrtc-topo-pulse 2s infinite; }',
                '.olcrtc-topo-title { font-weight: 600; font-size: 1.1em; margin-top: 10px; color: var(--text-color, inherit); }',
                '.olcrtc-topo-sub { font-size: 0.85em; opacity: 0.7; margin-top: 6px; text-align: center; line-height: 1.4; }',
                '.olcrtc-topo-icon { font-size: 2.2em; line-height: 1; display: flex; justify-content: center; align-items: center; width: 48px; height: 48px; background: rgba(128,128,128,0.08); border-radius: 12px; }',
                '.olcrtc-topo-arrow-wrapper { display: flex; flex-direction: column; align-items: center; opacity: 0.5; margin: 12px 0; }',
                '.olcrtc-topo-arrow-label { font-size: 0.8em; font-weight: 700; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 6px; }',
                '.olcrtc-topo-arrow { width: 2px; height: 20px; background: currentColor; position: relative; }',
                '.olcrtc-topo-arrow::after { content: ""; position: absolute; bottom: -4px; left: -4px; border-width: 5px 5px 0; border-style: solid; border-color: currentColor transparent transparent; }',
                '.olcrtc-topo-sb { background: rgba(33, 150, 243, 0.04); border: 1px solid rgba(33, 150, 243, 0.2); border-radius: 16px; padding: 0; width: 100%; max-width: 550px; overflow: hidden; }',
                '.olcrtc-topo-sb-header { background: rgba(33, 150, 243, 0.08); padding: 12px; font-weight: 600; color: #1976d2; display: flex; align-items: center; justify-content: center; gap: 8px; font-size: 1.1em; }',
                '.olcrtc-topo-rule { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid rgba(128,128,128,0.1); }',
                '.olcrtc-topo-rule:last-child { border-bottom: none; }',
                '.olcrtc-topo-rule-match { font-family: monospace; font-size: 0.9em; background: rgba(128,128,128,0.1); padding: 4px 8px; border-radius: 6px; font-weight: 600; }',
                '.olcrtc-topo-rule-target { font-size: 0.9em; display: flex; align-items: center; gap: 8px; font-weight: 500; opacity: 0.9; }',
                '.olcrtc-topo-dns { font-size: 0.85em; opacity: 0.6; font-weight: normal; }',
                '@keyframes olcrtc-topo-pulse { 0% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0.3); } 70% { box-shadow: 0 0 0 10px rgba(67, 160, 71, 0); } 100% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0); } }'
            ].join('\n'));
            document.head.appendChild(styleNode);
        }

        function arrowDown(label) {
            return E('div', { 'class': 'olcrtc-topo-arrow-wrapper' }, [
                label ? E('div', { 'class': 'olcrtc-topo-arrow-label' }, [label]) : null,
                E('div', { 'class': 'olcrtc-topo-arrow' })
            ]);
        }

        return E('div', {'class': 'cbi-section'}, [
            E('h3', {}, [_('Live Traffic Flow')]),

            E('div', { 'class': 'olcrtc-topo-container' }, [

                E('div', { 'class': 'olcrtc-topo-card' }, [
                    E('div', { 'class': 'olcrtc-topo-icon' }, ['\ud83d\udcbb']),
                    E('div', { 'class': 'olcrtc-topo-title' }, ['LAN (br-lan)']),
                    E('div', { 'class': 'olcrtc-topo-sub' }, [_('Client devices in local network')])
                ]),

                arrowDown('tproxy redirect'),

                E('div', { 'class': 'olcrtc-topo-sb' }, [
                    E('div', { 'class': 'olcrtc-topo-sb-header' }, [_('Sing-box')]),

                    E('div', { 'class': 'olcrtc-topo-rule' }, [
                        E('div', { 'class': 'olcrtc-topo-rule-match' }, ['.local, .lan']),
                        E('div', { 'class': 'olcrtc-topo-rule-target' }, ['\u27a4 DIRECT', E('span', { 'class': 'olcrtc-topo-dns' }, ['(DNS: Local)'])])
                    ]),

                    E('div', { 'class': 'olcrtc-topo-rule' }, [
                        E('div', { 'class': 'olcrtc-topo-rule-match' }, ['whitelist.json']),
                        E('div', { 'class': 'olcrtc-topo-rule-target' }, ['\u27a4 DIRECT', E('span', { 'class': 'olcrtc-topo-dns' }, ['(DNS: Yandex)'])])
                    ]),

                    E('div', { 'class': 'olcrtc-topo-rule' }, [
                        E('div', { 'class': 'olcrtc-topo-rule-match', 'style': 'background:transparent; padding:0;' }, ['\ud83c\udf10 ' + _('All other traffic')]),
                        E('div', { 'class': 'olcrtc-topo-rule-target' }, ['\u27a4 route-select'])
                    ])
                ]),

                arrowDown('selector: route-select'),

                E('div', { 'style': 'display: flex; gap: 24px; width: 100%; max-width: 550px; justify-content: center;' }, [

                    E('div', { 'class': 'olcrtc-topo-card ' + (activeRoute === 'proxy' ? 'active active-pulse' : ''), 'style': 'flex: 1;' }, [
                        E('div', { 'class': 'olcrtc-topo-icon' }, ['\ud83d\ude87']),
                        E('div', { 'class': 'olcrtc-topo-title' }, ['PROXY (olcRTC)']),
                        E('div', { 'class': 'olcrtc-topo-sub' }, ['SOCKS5 :1080', E('br'), 'DNS: Quad9'])
                    ]),

                    E('div', { 'class': 'olcrtc-topo-card ' + (activeRoute === 'direct' ? 'active' : ''), 'style': 'flex: 1;' }, [
                        E('div', { 'class': 'olcrtc-topo-icon' }, ['\ud83c\udf0d']),
                        E('div', { 'class': 'olcrtc-topo-title' }, ['DIRECT']),
                        E('div', { 'class': 'olcrtc-topo-sub' }, ['ISP / Internet', E('br'), 'DNS: Yandex'])
                    ])
                ])
            ])
        ]);
    },

    /* -- Rule Sets (Raw Textarea Whitelist) --------------- */
    _buildWhitelist: function (whitelist) {
        if (whitelist.domains) {
            this._wlDomains = whitelist.domains;
        } else if (whitelist.rules && whitelist.rules[0] && whitelist.rules[0].domain_suffix) {
            this._wlDomains = whitelist.rules[0].domain_suffix;
        } else {
            this._wlDomains = [];
        }

        var section = E('div', {'class': 'cbi-section'});
        section.appendChild(E('h3', {}, [_('Rule Sets (Bypass / Whitelist)')]));
        section.appendChild(E('p', {'class': 'cbi-section-descr'}, [
            '\ud83d\udcc4 /etc/sing-box/whitelist.json \u2014 ' + _('Enter one domain per line. These domains will bypass the proxy.')
        ]));

        var textarea = E('textarea', {
            'class': 'cbi-input-textarea',
            'style': 'width: 100%; height: 250px; font-family: monospace; resize: vertical; padding: 10px; border-radius: 4px;',
            'wrap': 'off'
        }, this._wlDomains.join('\n'));

        section.appendChild(textarea);

        section.appendChild(E('div', {'class': 'cbi-page-actions'}, [
            E('button', {
                'class': 'cbi-button cbi-button-apply important',
                'click': function () {
                    var lines = textarea.value.split('\n');
                    var domains = [];
                    for (var i = 0; i < lines.length; i++) {
                        var d = lines[i].trim();
                        if (d) domains.push(d);
                    }

                    domains = domains.filter(function(item, pos) {
                        return domains.indexOf(item) === pos;
                    }).sort();

                    textarea.value = domains.join('\n');

                    callSetWhitelist({domains: domains}).then(function () {
                        ui.addNotification(null, E('p', {}, [_('Whitelist saved and sing-box reloaded')]), 'info');
                    }).catch(function (err) {
                        ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
                    });
                }
            }, ['\ud83d\udcbe ' + _('Save & Reload Sing-box')])
        ]));

        return section;
    },

    /* -- Manual Routing Override -------------------------- */
    _buildRouteOverride: function () {
        var self = this;
        var current = this._route;

        function radio(value, label, hint) {
            var r = E('input', {
                'type': 'radio', 'name': 'route-override', 'value': value,
                'change': function () {
                    if (value === 'auto') {
                        ui.addNotification(null, E('p', {},
                            [_('Bot controls the route automatically (auto-promotes proxy on stable > 30s).')]), 'info');
                        return;
                    }
                    callSetRoute({mode: value}).then(function () {
                        ui.addNotification(null, E('p', {}, [_('Route switched to') + ' ' + value]), 'info');
                        self._route = value;
                    }).catch(function (err) {
                        ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
                    });
                }
            });
            if ((value === 'auto' && (current !== 'proxy' && current !== 'direct')) || value === current) r.checked = true;
            return E('label', {'style': 'margin-right: 18px; cursor: pointer;'}, [
                r, ' ', E('span', {'style': 'font-weight: bold;'}, [label]), ' ',
                E('span', {'style': 'opacity: 0.7; font-size: 0.8em;'}, [hint])
            ]);
        }

        return E('div', {'class': 'cbi-section'}, [
            E('h3', {}, [_('Manual Routing Override')]),
            E('div', {'style': 'margin: 10px 0;'}, [
                radio('auto', '\ud83e\udd16 ' + _('Auto (Bot)'), ''),
                radio('direct', '\ud83c\udf0d ' + _('Direct'), ''),
                radio('proxy', '\ud83d\ude87 ' + _('Proxy'), '')
            ]),
            E('p', {'class': 'cbi-section-descr'}, [
                '\u2139\ufe0f ' + _('Direct/Proxy apply immediately via the sing-box selector; Auto is cosmetic \u2014 the bot still auto-promotes proxy when stable.')
            ])
        ]);
    },

    /* -- Diagnostics (firewall) --------------------------- */
    _buildDiagnostics: function (data) {
        var nftActive = data.nft_chain && data.nft_chain.length > 0;
        var ruleCount = data.nft_chain ? (data.nft_chain.match(/rule/g) || []).length : 0;
        var ipRuleOk = data.ip_rule === 'present';
        var listening = data.listening === true || data.listening === 'true';

        return E('details', {'class': 'cbi-section', 'style': 'margin-top: 12px;'}, [
            E('summary', {'style': 'cursor: pointer; font-weight: bold; color: #1976d2;'}, [_('Diagnostics & Connection Tracking (collapsed)')]),
            E('table', {'class': 'table', 'width': '100%', 'style': 'margin-top: 8px;'}, [
                E('tr', {}, [E('td', {'width': '40%'}, ['nft chain singbox_tproxy']),
                    E('td', {}, [statusEmoji(nftActive), ' ', nftActive ? _('Active') + ' (' + ruleCount + ' ' + _('rules') + ')' : _('Inactive')])]),
                E('tr', {}, [E('td', {}, ['ip rule fwmark 0x1']),
                    E('td', {}, [statusEmoji(ipRuleOk), ' ', ipRuleOk ? _('Present') : _('Absent')])]),
                E('tr', {}, [E('td', {}, ['sing-box :2080']),
                    E('td', {}, [statusEmoji(listening), ' ', listening ? _('Listening') : _('Not listening')])]),
                E('tr', {}, [E('td', {}, [_('Conntrack')]),
                    E('td', {}, ['' + (data.conntrack_count || 0)])])
            ]),
            E('p', {'class': 'cbi-section-descr'}, [
                '\u21ba ' + _('Use Service \u2192 Reload on the System page if these checks fail.')
            ])
        ]);
    },

    _setupPoll: function () {
        var self = this;
        if (this._pollFn) poll.remove(this._pollFn);
        this._pollFn = function () {
            return Promise.all([callStatus(), callGetSingbox()]).then(function (res) {
                if (!self._topologySection || !self._topologySection.parentNode) return;
                var fresh = self._buildTopology(res[0] || {}, res[1] || {});
                self._topologySection.parentNode.replaceChild(fresh, self._topologySection);
                self._topologySection = fresh;
                self._route = (res[1] || {}).now || (res[1] || {}).route || 'unknown';
            }).catch(function () {});
        };
        poll.add(this._pollFn, 5);
    }
});
