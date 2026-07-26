'use strict';
'require view';
'require fs';
'require uci';
'require tapx.common as tapx';

var ZH = {
	coreStatus: 'TapX \u6838\u5fc3', panelStatus: 'TapX-UI', info: '\u8fd0\u884c\u4fe1\u606f', panelUrl: '\u9762\u677f\u5730\u5740', certificate: '\u8bc1\u4e66\u8def\u5f84', privateKey: '\u79c1\u94a5\u8def\u5f84',
	coreVersion: 'TapX \u7248\u672c', panelVersion: 'TapX-UI \u7248\u672c', runtimeConfig: 'TapX \u8fd0\u884c\u914d\u7f6e', database: '\u6570\u636e\u5e93\u8def\u5f84', uciConfig: 'OpenWrt \u914d\u7f6e', notConfigured: '\u672a\u914d\u7f6e'
};
var EN = {
	coreStatus: 'TapX Core', panelStatus: 'TapX-UI', info: 'Runtime information', panelUrl: 'Panel address', certificate: 'Certificate path', privateKey: 'Private key path',
	coreVersion: 'TapX version', panelVersion: 'TapX-UI version', runtimeConfig: 'TapX runtime configuration', database: 'Database path', uciConfig: 'OpenWrt configuration', notConfigured: 'Not configured'
};
var tr = tapx.translator(ZH, EN);
function value(text) { return text || tr('notConfigured'); }
function row(label, content) { return E('div', { 'class': 'tapx-detail-row' }, [ E('dt', {}, label), E('dd', {}, content) ]); }

return view.extend({
	load: function() {
		return Promise.all([
			uci.load('tapx'),
			L.resolveDefault(fs.exec(tapx.PIDOF, [ 'tapx-core' ]), { code: 1 }),
			L.resolveDefault(fs.exec(tapx.PIDOF, [ 'tapx-panel' ]), { code: 1 }),
			L.resolveDefault(fs.exec('/usr/bin/tapx-core', [ '-version' ]), { code: 1 }),
			L.resolveDefault(fs.exec('/usr/bin/tapx-panel', [ '-version' ]), { code: 1 })
		]);
	},
	render: function(data) {
		var initialized = uci.get('tapx', 'panel', 'initialized') === '1';
		var port = uci.get('tapx', 'panel', 'listen_port') || '';
		var basePath = uci.get('tapx', 'panel', 'base_path') || '';
		var panelUrl = initialized && port && basePath ? tapx.panelUrl(port, basePath, uci.get('tapx', 'panel', 'https') === '1') : '';
		var panelLink = panelUrl ? E('a', { 'href': panelUrl, 'target': '_blank', 'rel': 'noopener noreferrer' }, panelUrl) : tr('notConfigured');

		return E('div', { 'class': 'tapx-page' }, [
			tapx.styles(),
			E('section', { 'class': 'tapx-plain-section' }, [
				E('h3', { 'class': 'tapx-section-title' }, tr('info')),
				E('dl', { 'class': 'tapx-details' }, [
					row(tr('coreStatus'), tapx.status(data[1].code === 0)),
					row(tr('panelStatus'), tapx.status(data[2].code === 0)),
					row(tr('coreVersion'), value(tapx.output(data[3]))),
					row(tr('panelVersion'), value(tapx.output(data[4]))),
					row(tr('panelUrl'), panelLink),
					row(tr('certificate'), value(uci.get('tapx', 'panel', 'cert_file'))),
					row(tr('privateKey'), value(uci.get('tapx', 'panel', 'key_file'))),
					row(tr('runtimeConfig'), value(uci.get('tapx', 'core', 'config_path') || '/etc/tapx/runtime.json')),
					row(tr('database'), value(uci.get('tapx', 'panel', 'db_path') || '/etc/tapx/tapx.db')),
					row(tr('uciConfig'), '/etc/config/tapx')
				])
			])
		]);
	},
	handleSaveApply: null, handleSave: null, handleReset: null
});
