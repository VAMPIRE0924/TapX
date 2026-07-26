'use strict';
'require view';
'require fs';
'require uci';
'require ui';
'require tapx.common as tapx';

var CONFIG_HELPER = '/usr/libexec/tapx-openwrt-config';
var RESTORE_UPLOAD = '/tmp/tapx-openwrt-restore.tar.gz';
var ZH = { subtitle: '\u5bfc\u51fa\u3001\u6062\u590d\u6216\u91cd\u7f6e TapX \u6570\u636e', exportTitle: '\u5bfc\u51fa\u914d\u7f6e', exportText: '\u5305\u542b TapX \u6570\u636e\u5e93\u548c\u9762\u677f\u8bbe\u7f6e\uff0c\u4e0d\u5305\u542b\u8bc1\u4e66\u3002', exportButton: '\u4e0b\u8f7d\u5907\u4efd', restoreTitle: '\u6062\u590d\u914d\u7f6e', restoreText: '\u4e0a\u4f20 TapX OpenWrt \u5907\u4efd\u5305\u5e76\u66ff\u6362\u5f53\u524d\u6570\u636e\u3002', restoreButton: '\u9009\u62e9\u5907\u4efd\u5305', resetTitle: '\u56fa\u4ef6\u9ed8\u8ba4\u503c', resetText: '\u6062\u590d\u7f16\u8bd1\u5230\u56fa\u4ef6\u4e2d\u7684 TapX \u521d\u59cb\u914d\u7f6e\u3002', resetButton: '\u91cd\u7f6e TapX', setupFirst: '\u8bf7\u5148\u5b8c\u6210 TapX-UI \u521d\u59cb\u5316', restoreConfirm: '\u6062\u590d\u4f1a\u66ff\u6362\u5f53\u524d TapX \u6570\u636e\uff0c\u662f\u5426\u7ee7\u7eed\uff1f', resetConfirm: '\u786e\u5b9a\u6062\u590d\u56fa\u4ef6\u5185\u7f6e\u7684 TapX \u914d\u7f6e\uff1f', restored: 'TapX \u914d\u7f6e\u5df2\u6062\u590d', resetDone: '\u5df2\u6062\u590d\u56fa\u4ef6\u5185\u7f6e\u914d\u7f6e' };
var EN = { subtitle: 'Export, restore, or reset TapX data', exportTitle: 'Export configuration', exportText: 'Includes the TapX database and panel settings. Certificates are excluded.', exportButton: 'Download backup', restoreTitle: 'Restore configuration', restoreText: 'Upload a TapX OpenWrt backup and replace the current data.', restoreButton: 'Choose backup', resetTitle: 'Firmware defaults', resetText: 'Restore the TapX configuration compiled into the firmware.', resetButton: 'Reset TapX', setupFirst: 'Initialize TapX-UI first', restoreConfirm: 'Restore replaces the current TapX data. Continue?', resetConfirm: 'Restore the TapX configuration built into this firmware?', restored: 'TapX configuration restored', resetDone: 'Firmware defaults restored' };
var tr = tapx.translator(ZH, EN);

return view.extend({
	load: function() { return uci.load('tapx'); },
	render: function() {
		var initialized = uci.get('tapx', 'panel', 'initialized') === '1';
		function exportConfig() { if (!initialized) { tapx.notify(tr('setupFirst'), 'warning'); return; } return fs.exec_direct(CONFIG_HELPER, [ 'export' ], 'blob').then(function(blob) { var url = window.URL.createObjectURL(blob), anchor = E('a', { 'href': url, 'download': 'tapx-openwrt-backup.tar.gz' }); document.body.appendChild(anchor); anchor.click(); anchor.remove(); window.setTimeout(function() { window.URL.revokeObjectURL(url); }, 1000); }).catch(function(error) { tapx.notify(error.message || String(error), 'danger'); }); }
		function restoreConfig() { return ui.uploadFile(RESTORE_UPLOAD, null, tr('restoreText')).then(function() { if (!window.confirm(tr('restoreConfirm'))) return fs.remove(RESTORE_UPLOAD); return tapx.run(CONFIG_HELPER, [ 'restore', RESTORE_UPLOAD ]).then(function() { tapx.notify(tr('restored')); window.setTimeout(function() { window.location.reload(); }, 600); }); }).catch(function(error) { tapx.notify(error.message || String(error), 'danger'); }); }
		function resetConfig() { if (!window.confirm(tr('resetConfirm'))) return; return tapx.run(CONFIG_HELPER, [ 'reset' ]).then(function() { tapx.notify(tr('resetDone')); window.setTimeout(function() { window.location.reload(); }, 600); }).catch(function(error) { tapx.notify(error.message || String(error), 'danger'); }); }
		return E('div', { 'class': 'tapx-page' }, [ tapx.styles(), tapx.pageHeader(tapx.tr('backup'), tr('subtitle')), E('div', { 'class': 'tapx-action-list' }, [
			E('section', { 'class': 'tapx-card' }, [ E('h3', {}, tr('exportTitle')), E('p', {}, tr('exportText')), tapx.button(tr('exportButton'), 'action', exportConfig) ]),
			E('section', { 'class': 'tapx-card' }, [ E('h3', {}, tr('restoreTitle')), E('p', {}, tr('restoreText')), tapx.button(tr('restoreButton'), 'reload', restoreConfig) ]),
			E('section', { 'class': 'tapx-card' }, [ E('h3', {}, tr('resetTitle')), E('p', {}, tr('resetText')), tapx.button(tr('resetButton'), 'reset', resetConfig) ])
		]) ]);
	},
	handleSaveApply: null, handleSave: null, handleReset: null
});
