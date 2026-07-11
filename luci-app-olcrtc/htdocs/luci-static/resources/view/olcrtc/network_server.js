'use strict';
'require rpc';
'require ui';
'require baseclass';

var callGetEnv = rpc.declare({object: 'olcrtc-bot', method: 'get_env', expect: {}});
var callSetEnv = rpc.declare({object: 'olcrtc-bot', method: 'set_env', params: ['env', 'restart']});
var callTestSocksProxy = rpc.declare({object: 'olcrtc-bot', method: 'test_socks_proxy', params: ['addr', 'port', 'user', 'pass']});

return baseclass.extend({
    load: function () {
        return Promise.all([callGetEnv()]);
    },

    render: function (container, data) {
        var env = data[0] || {};
        container.appendChild(this._buildEgressSection(env));
    },

    /* -- Upstream SOCKS5 Egress (server mode) ------------- */
    _buildEgressSection: function (env) {
        env = env || {};
        var curAddr = env.SOCKS_PROXY_ADDR || '';
        var curPort = env.SOCKS_PROXY_PORT || '';
        var curUser = env.SOCKS_PROXY_USER || '';
        var curPass = env.SOCKS_PROXY_PASS || '';

        var section = E('div', {'class': 'cbi-section'});
        section.appendChild(E('h3', {}, [_('Upstream SOCKS5 Egress')]));

        var curRow;
        if (curAddr) {
            var authLabel;
            if (curUser && curPass) {
                authLabel = _('user / password');
            } else if (curUser) {
                authLabel = _('user / *(none)*');
            } else {
                authLabel = _('none');
            }
            curRow = E('p', {'style': 'font-size:1.05em;margin:8px 0 4px 0;'}, [
                _('Current egress:') + ' \ud83d\udfe2 ' + _('SOCKS5') + '  \u2192  ' + curAddr + ' : ' + curPort,
                E('br'),
                _('Authentication:') + ' ' + authLabel
            ]);
        } else {
            curRow = E('p', {'style': 'font-size:1.05em;margin:8px 0 4px 0;'}, [
                _('Current egress:') + ' \ud83d\udd34 ' + _('DIRECT') + '  (' + _('no upstream proxy') + ')'
            ]);
        }
        section.appendChild(curRow);

        var addrInput = E('input', {
            'class': 'cbi-input-text',
            'type': 'text',
            'value': curAddr,
            'placeholder': '203.0.113.10'
        });
        var portInput = E('input', {
            'class': 'cbi-input-text',
            'type': 'text',
            'value': curPort,
            'placeholder': '1080'
        });
        var userInput = E('input', {
            'class': 'cbi-input-text',
            'type': 'text',
            'value': curUser,
            'placeholder': _('optional')
        });
        var passInput = E('input', {
            'class': 'cbi-input-password',
            'type': 'password',
            'value': curPass,
            'placeholder': _('optional')
        });

        section.appendChild(E('table', {'class': 'table'}, [
            E('tr', {}, [
                E('td', {'width': '30%'}, [_('Proxy address') + ' *']),
                E('td', {}, [addrInput])
            ]),
            E('tr', {}, [
                E('td', {}, [_('Proxy port') + ' *']),
                E('td', {}, [portInput])
            ]),
            E('tr', {}, [
                E('td', {}, [_('Username')]),
                E('td', {}, [userInput])
            ]),
            E('tr', {}, [
                E('td', {}, [_('Password')]),
                E('td', {}, [passInput, ' ', this._buildPassToggle(passInput)])
            ])
        ]));

        section.appendChild(E('p', {'class': 'cbi-section-descr'}, [
            '\u23f0 ' + _('Empty "Proxy address" = direct egress (no upstream proxy).'),
            E('br'),
            _('Empty "Username" = no-auth SOCKS5 (RFC 1929 method 0x00).'),
            E('br'),
            _('Saving restarts dial-up to regenerate srv.yaml.')
        ]));

        var testResult = E('p', {'style': 'margin:8px 0; font-weight:600; min-height:1.2em;'});

        function validatePort(addr, port) {
            if (!addr) return true;
            var n = Number(port);
            return port !== '' && /^\d+$/.test(port) && n >= 1 && n <= 65535;
        }

        section.appendChild(E('div', {'class': 'cbi-page-actions'}, [
            E('button', {
                'class': 'cbi-button',
                'click': function () {
                    var addr = addrInput.value.trim();
                    var port = portInput.value.trim();
                    if (addr && !validatePort(addr, port)) {
                        ui.addNotification(null, E('p', {}, [_('Port must be a number 1..65535 when address is set.')]), 'error');
                        return;
                    }
                    testResult.textContent = '\u23f3 ' + _('Testing...');
                    testResult.style.color = '#888';
                    callTestSocksProxy({addr: addr, port: port, user: userInput.value.trim(), pass: passInput.value}).then(function (res) {
                        res = res || {};
                        if (res.reachable) {
                            testResult.textContent = '\u2713 ' + _('reachable') + ' \u00b7 ' + res.rtt_ms + ' ms';
                            testResult.style.color = '#43a047';
                        } else {
                            testResult.textContent = '\u2717 ' + (res.error || _('unreachable'));
                            testResult.style.color = '#e53935';
                        }
                    }).catch(function (err) {
                        testResult.textContent = '\u2717 ' + (err.message || err);
                        testResult.style.color = '#e53935';
                    });
                }
            }, ['\ud83d\udd0c ' + _('Test')]),
            E('button', {
                'class': 'cbi-button cbi-button-apply important',
                'click': function () {
                    var addr = addrInput.value.trim();
                    var port = portInput.value.trim();
                    var user = userInput.value.trim();
                    var pass = passInput.value;
                    if (addr && !validatePort(addr, port)) {
                        ui.addNotification(null, E('p', {}, [_('Port must be a number 1..65535 when address is set.')]), 'error');
                        return;
                    }
                    callSetEnv({
                        SOCKS_PROXY_ADDR: addr,
                        SOCKS_PROXY_PORT: port,
                        SOCKS_PROXY_USER: user,
                        SOCKS_PROXY_PASS: pass
                    }, true).then(function () {
                        ui.addNotification(null, E('p', {}, [_('SOCKS5 egress saved and dial-up restarted.')]), 'info');
                    }).catch(function (err) {
                        ui.addNotification(null, E('p', {}, [_('Error:') + ' ' + (err.message || err)]), 'error');
                    });
                }
            }, ['\ud83d\udcbe ' + _('Save & Restart')])
        ]));

        section.appendChild(testResult);
        return section;
    },

    _buildPassToggle: function (passInput) {
        var toggle = E('span', {
            'style': 'cursor:pointer; user-select:none;',
            'click': function () {
                if (passInput.type === 'password') {
                    passInput.type = 'text';
                    toggle.textContent = '\ud83d\udc41';
                } else {
                    passInput.type = 'password';
                    toggle.textContent = '\ud83d\udd12';
                }
            }
        }, ['\ud83d\udd12']);
        return toggle;
    }
});
