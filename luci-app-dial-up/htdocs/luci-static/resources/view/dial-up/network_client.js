'use strict';
'require rpc';
'require ui';
'require poll';
'require baseclass';

var callStatus = rpc.declare({object: 'dial-up-bot', method: 'get_status', expect: {}});
var callGetSingbox = rpc.declare({object: 'dial-up-bot', method: 'get_singbox', expect: {}});
var callSetRoute = rpc.declare({object: 'dial-up-bot', method: 'set_route', params: ['mode']});
var callGetFirewall = rpc.declare({object: 'dial-up-bot', method: 'get_firewall', expect: {}});
var callGetWhitelist = rpc.declare({object: 'dial-up-bot', method: 'get_whitelist', expect: {}});
var callSetWhitelist = rpc.declare({object: 'dial-up-bot', method: 'set_whitelist', params: ['domains']});

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
        container.appendChild(this._buildDiagnostics(firewall));
        this._setupPoll();
    },

    /* -- Live Traffic Flow topology (Modern UI) ---------- */
    _buildTopology: function (status, singbox) {
        var self = this;
        var activeRoute = singbox.now || singbox.route || 'unknown';
        var awgPresent = singbox.awg_present === true || singbox.awg_present === 'true';
        var awgOnline = singbox.awg_online === true || singbox.awg_online === 'true';
        var awgIface = singbox.awg_iface || '';

        if (!document.getElementById('dial-up-topo-styles')) {
            var styleNode = E('style', {'id': 'dial-up-topo-styles'}, [
                '.dial-up-topo-container { display: grid; grid-template-columns: 1fr auto 1fr; align-items: start; gap: 22px; padding: 8px 0 12px 0; width: 100%; max-width: 760px; margin: 0 auto; }',
                '.dial-up-topo-col { display: flex; flex-direction: column; align-items: center; gap: 9px; min-width: 0; }',
                '.dial-up-topo-divider { width: 2px; align-self: stretch; background: rgba(128,128,128,0.15); border-radius: 2px; }',
                '.dial-up-topo-card { background: rgba(128,128,128,0.06); border: 2px solid rgba(128,128,128,0.12); border-radius: 14px; padding: 14px 18px; min-width: 200px; display: flex; flex-direction: row; align-items: center; gap: 12px; transition: all 0.3s ease; box-shadow: 0 3px 8px rgba(0,0,0,0.03); }',
                '.dial-up-topo-card.active { border-color: #43a047; background: rgba(67, 160, 71, 0.08); }',
                '.dial-up-topo-card.active-pulse { animation: dial-up-topo-pulse 2s infinite; }',
                '.dial-up-topo-card.disabled { opacity: 0.45; cursor: not-allowed; filter: grayscale(0.6); }',
                '.dial-up-topo-card.selectable { cursor: pointer; transition: all 0.2s ease; }',
                '.dial-up-topo-card.selectable:hover { border-color: rgba(67, 160, 71, 0.5); box-shadow: 0 6px 16px rgba(67, 160, 71, 0.18); transform: translateY(-2px); }',
                '.dial-up-topo-card-text { display: flex; flex-direction: column; align-items: flex-start; text-align: left; }',
                '.dial-up-topo-title { font-weight: 600; font-size: 1.05em; color: var(--text-color, inherit); }',
                '.dial-up-topo-sub { font-size: 0.85em; opacity: 0.7; margin-top: 3px; text-align: left; line-height: 1.3; }',
                '.dial-up-topo-icon { font-size: 2em; line-height: 1; display: flex; justify-content: center; align-items: center; width: 42px; height: 42px; background: rgba(128,128,128,0.08); border-radius: 10px; flex-shrink: 0; }',
                '.dial-up-topo-arrow-wrapper { display: flex; flex-direction: column; align-items: center; opacity: 0.5; margin: 0; padding: 3px 0; min-height: 33px; justify-content: flex-start; }',
                '.dial-up-topo-arrow-label { font-size: 0.75em; font-weight: 700; text-transform: uppercase; letter-spacing: 0.4px; margin-bottom: 2px; white-space: nowrap; }',
                '.dial-up-topo-arrow { width: 2px; height: 16px; background: currentColor; position: relative; }',
                '.dial-up-topo-arrow::after { content: ""; position: absolute; bottom: -4px; left: -4px; border-width: 5px 5px 0; border-style: solid; border-color: currentColor transparent transparent; }',
                '.dial-up-topo-sb { background: rgba(33, 150, 243, 0.04); border: 1px solid rgba(33, 150, 243, 0.2); border-radius: 12px; padding: 0; width: 100%; overflow: hidden; }',
                '.dial-up-topo-sb-header { background: rgba(33, 150, 243, 0.08); padding: 8px 12px; font-weight: 600; color: #1976d2; display: flex; align-items: center; justify-content: center; gap: 6px; font-size: 1em; }',
                '.dial-up-topo-rule { display: flex; align-items: center; justify-content: space-between; padding: 8px 12px; border-bottom: 1px solid rgba(128,128,128,0.1); }',
                '.dial-up-topo-rule:last-child { border-bottom: none; }',
                '.dial-up-topo-rule-match { font-family: monospace; font-size: 0.8em; background: rgba(128,128,128,0.1); padding: 2px 6px; border-radius: 6px; font-weight: 600; white-space: nowrap; flex-shrink: 0; }',
                '.dial-up-topo-rule-target { font-size: 0.85em; display: flex; align-items: center; gap: 6px; font-weight: 500; opacity: 0.9; white-space: nowrap; flex-shrink: 0; }',
                '.dial-up-topo-dns { font-size: 0.76em; opacity: 0.6; font-weight: normal; }',
                '.dial-up-topo-badge { display: inline-block; margin-top: 4px; padding: 2px 10px; border-radius: 999px; font-size: 0.78em; font-weight: 700; text-transform: uppercase; letter-spacing: 0.4px; white-space: nowrap; }',
                '.dial-up-topo-badge.badge-ok { background: rgba(67,160,71,0.16); color: #43a047; border: 1px solid rgba(67,160,71,0.45); }',
                '.dial-up-topo-badge.badge-bad { background: rgba(229,57,53,0.13); color: #e53935; border: 1px solid rgba(229,57,53,0.45); }',
                '@keyframes dial-up-topo-pulse { 0% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0.3); } 70% { box-shadow: 0 0 0 10px rgba(67, 160, 71, 0); } 100% { box-shadow: 0 0 0 0 rgba(67, 160, 71, 0); } }'
            ].join('\n'));
            document.head.appendChild(styleNode);
        }

        function arrowDown(label) {
            return E('div', { 'class': 'dial-up-topo-arrow-wrapper' }, [
                label ? E('div', { 'class': 'dial-up-topo-arrow-label' }, [label]) : null,
                E('div', { 'class': 'dial-up-topo-arrow' })
            ]);
        }

        function statusBadge(ok, text) {
            return E('span', { 'class': 'dial-up-topo-badge ' + (ok ? 'badge-ok' : 'badge-bad') }, [text]);
        }

        return E('div', {'class': 'cbi-section'}, [
            E('h3', {}, [_('Live Traffic Flow')]),

            E('div', { 'class': 'dial-up-topo-container' }, [

                E('div', { 'class': 'dial-up-topo-col' }, [

                    arrowDown('tproxy redirect'),

                    E('div', { 'class': 'dial-up-topo-sb' }, [
                        E('div', { 'class': 'dial-up-topo-sb-header' }, [_('Sing-box')]),

                        E('div', { 'class': 'dial-up-topo-rule' }, [
                            E('div', { 'class': 'dial-up-topo-rule-match' }, ['local, .lan, .internal']),
                            E('div', { 'class': 'dial-up-topo-rule-target' }, ['\u27a4 DIRECT', E('span', { 'class': 'dial-up-topo-dns' }, ['(DNS: Local)'])])
                        ]),

                        E('div', { 'class': 'dial-up-topo-rule' }, [
                            E('div', { 'class': 'dial-up-topo-rule-match' }, ['whitelist.json']),
                            E('div', { 'class': 'dial-up-topo-rule-target' }, ['\u27a4 DIRECT', E('span', { 'class': 'dial-up-topo-dns' }, ['(DNS: Yandex)'])])
                        ]),

                        E('div', { 'class': 'dial-up-topo-rule' }, [
                            E('div', { 'class': 'dial-up-topo-rule-match', 'style': 'background:transparent; padding:0;' }, ['\ud83c\udf10 ' + _('All other traffic')]),
                            E('div', { 'class': 'dial-up-topo-rule-target' }, ['\u27a4 route-select'])
                        ])
                    ])
                ]),

                E('div', { 'class': 'dial-up-topo-divider' }),

                E('div', { 'class': 'dial-up-topo-col' }, [

                    arrowDown('selector: route-select'),

                    E('div', {
                        'class': 'dial-up-topo-card selectable ' + (activeRoute === 'proxy' ? 'active active-pulse' : ''),
                        'style': 'width: 100%; max-width: 260px;',
                        'title': _('Click to route all traffic through proxy'),
                        'click': function () {
                            callSetRoute('proxy').then(function () {
                                ui.addNotification(null, E('p', {}, [_('Route switched to') + ' proxy']), 'info');
                                self._route = 'proxy';
                            }).catch(function (err) {
                                ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
                            });
                        }
                    }, [
                        E('div', { 'class': 'dial-up-topo-icon' }, ['\ud83d\ude87']),
                        E('div', { 'class': 'dial-up-topo-card-text' }, [
                            E('div', { 'class': 'dial-up-topo-title' }, ['PROXY']),
                            E('div', { 'class': 'dial-up-topo-sub' }, ['OlcRTC SOCKS5 :1080', E('br'), 'DNS: remote'])
                        ])
                    ]),

                    E('div', {
                        'class': 'dial-up-topo-card selectable ' + (activeRoute === 'awg' ? 'active' : '') + (awgPresent ? '' : ' disabled'),
                        'style': 'width: 100%; max-width: 260px;',
                        'title': awgPresent
                            ? (awgOnline ? _('AWG online — click to route via AmneziaVPN') : _('AWG offline — click to route via AmneziaVPN'))
                            : _('AmneziaVPN is not configured on this router'),
                        'click': awgPresent ? function () {
                            callSetRoute('awg').then(function () {
                                ui.addNotification(null, E('p', {}, [_('Route switched to') + ' awg']), 'info');
                                self._route = 'awg';
                            }).catch(function (err) {
                                ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
                            });
                        } : null
                    }, [
                        E('div', { 'class': 'dial-up-topo-icon' }, ['🔐']),
                        E('div', { 'class': 'dial-up-topo-card-text' }, [
                            E('div', { 'class': 'dial-up-topo-title' }, ['AMNEZIA VPN']),
                            awgPresent
                                ? statusBadge(awgOnline, awgOnline ? _('online') : _('offline'))
                                : statusBadge(false, _('not configured')),
                            E('div', { 'class': 'dial-up-topo-sub' }, [
                                'Interface ' + (awgIface ? awgIface : '\u2014')
                            ])
                        ])
                    ]),

                    E('div', {
                        'class': 'dial-up-topo-card selectable ' + (activeRoute === 'direct' ? 'active' : ''),
                        'style': 'width: 100%; max-width: 260px;',
                        'title': _('Click to route all traffic directly (bypass proxy)'),
                        'click': function () {
                            callSetRoute('direct').then(function () {
                                ui.addNotification(null, E('p', {}, [_('Route switched to') + ' direct']), 'info');
                                self._route = 'direct';
                            }).catch(function (err) {
                                ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
                            });
                        }
                    }, [
                        E('div', { 'class': 'dial-up-topo-icon' }, ['\ud83c\udf0d']),
                        E('div', { 'class': 'dial-up-topo-card-text' }, [
                            E('div', { 'class': 'dial-up-topo-title' }, ['DIRECT']),
                            E('div', { 'class': 'dial-up-topo-sub' }, [
                                'ISP / Internet',
                                E('br'),
                                'DNS: Yandex'
                            ])
                        ])
                    ])
                ])
            ]),

            E('p', {'style': 'text-align: center; opacity: 0.6; font-size: 0.85em; margin-top: 12px;'}, [
                '\ud83d\udc49 ' + _('Click PROXY or DIRECT to override the route. The bot auto-promotes proxy when stable.')
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
            }, ['\ud83d\udcbe ' + _('Save & Restart Sing-Box')])
        ]));

        return section;
    },

    /* -- Diagnostics (firewall) --------------------------- */
    _buildDiagnostics: function (data) {
        var nftActive = data.nft_chain && data.nft_chain.length > 0;
        var ruleCount = data.nft_chain ? (data.nft_chain.match(/rule/g) || []).length : 0;
        var ipRuleOk = data.ip_rule === 'present';
        var listening = data.listening === true || data.listening === 'true';

        return E('details', {'class': 'cbi-section', 'style': 'margin-top: 12px;'}, [
            E('summary', {'style': 'cursor: pointer; font-weight: bold; color: #1976d2;'}, [_('Diagnostics & Connection Tracking')]),
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
