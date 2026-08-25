'use strict';
'require rpc';
'require view';

var callStatus = rpc.declare({object: 'dial-up-bot', method: 'get_status', expect: {}});

return view.extend({
    load: function () {
        return Promise.all([
            L.require('view/dial-up/statusbar'),
            callStatus()
        ]);
    },

    render: function (data) {
        var statusbar = data[0];
        var status = data[1] || {};
        var isClient = status.is_client === true;

        var container = E('div', {'id': 'dial-up-network'});
        statusbar.mount(container);

        var modulePath = isClient
            ? 'view/dial-up/network_client'
            : 'view/dial-up/network_server';

        return L.require(modulePath).then(function (subview) {
            return subview.load().then(function (subData) {
                subview.render(container, subData);
                return container;
            });
        });
    },

    handleSave: null,
    handleSaveApply: null,
    handleReset: null
});
