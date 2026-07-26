'use strict';
'require view';
'require fs';
'require network';
'require uci';
'require tapx.common as tapx';

var PANEL_BIN = '/usr/bin/tapx-panel';
var CONFIG_HELPER = '/usr/libexec/tapx-openwrt-config';
var PASSWORD_UPLOAD = '/tmp/tapx-openwrt-panel-password';

var ZH = {
	access: '\u8bbf\u95ee\u8bbe\u7f6e', luci: 'LuCI \u914d\u7f6e', language: '\u8bed\u8a00', runtime: '\u8fd0\u884c\u8bbe\u7f6e', credentials: '\u767b\u5f55\u51ed\u636e',
	iface: '\u76d1\u542c\u7f51\u53e3', all: '\u6240\u6709\u63a5\u53e3', port: '\u9762\u677f\u7aef\u53e3', path: '\u767b\u5f55\u5165\u53e3', cert: '\u8bc1\u4e66\u8def\u5f84', key: '\u79c1\u94a5\u8def\u5f84',
	username: '\u7528\u6237\u540d', password: '\u5bc6\u7801', confirm: '\u786e\u8ba4\u5bc6\u7801', configured: '\u767b\u5f55\u51ed\u636e\u5df2\u914d\u7f6e', reset: '\u91cd\u8bbe\u51ed\u636e',
	save: '\u4fdd\u5b58\u5e76\u5e94\u7528', open: '\u6253\u5f00 TapX-UI', saved: '\u9ad8\u7ea7\u8bbe\u7f6e\u5df2\u4fdd\u5b58',
	ifaceHint: '\u7559\u7a7a\u76d1\u542c\u6240\u6709\u7f51\u53e3\u3002', portHint: '\u4f8b\u5982 2053\u3002', pathHint: '\u5fc5\u987b\u4ee5 / \u5f00\u5934\u548c\u7ed3\u5c3e\u3002', certHint: 'PEM \u8bc1\u4e66\u6587\u4ef6\u3002', keyHint: '\u4e0e\u8bc1\u4e66\u5339\u914d\u7684 PEM \u79c1\u94a5\u3002', userHint: '\u4fdd\u5b58\u540e\u4e0d\u518d\u663e\u793a\u3002',
	invalidPort: '\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535', invalidPath: '\u767b\u5f55\u5165\u53e3\u5fc5\u987b\u4ee5 / \u5f00\u5934\u5e76\u4ee5 / \u7ed3\u5c3e', certPair: 'HTTPS \u9700\u8981\u540c\u65f6\u586b\u5199\u8bc1\u4e66\u548c\u79c1\u94a5', needUser: '\u8bf7\u8f93\u5165\u7528\u6237\u540d', needPassword: '\u8bf7\u8f93\u5165\u5bc6\u7801', mismatch: '\u4e24\u6b21\u5bc6\u7801\u4e0d\u4e00\u81f4', unavailable: '\u7aef\u53e3\u4e0d\u53ef\u7528\uff1a{error}'
};
var EN = {
	access: 'Access', luci: 'LuCI configuration', language: 'Language', runtime: 'Runtime', credentials: 'Credentials', iface: 'Listening interface', all: 'All interfaces', port: 'Panel port', path: 'Login path', cert: 'Certificate path', key: 'Private key path', username: 'Username', password: 'Password', confirm: 'Confirm password', configured: 'Credentials configured', reset: 'Reset credentials', save: 'Save and apply', open: 'Open TapX-UI', saved: 'Advanced settings saved', ifaceHint: 'Leave blank to listen on every interface.', portHint: 'For example: 2053.', pathHint: 'Must start and end with /.', certHint: 'PEM certificate file.', keyHint: 'Matching PEM private key.', userHint: 'Not shown after saving.', invalidPort: 'Port must be between 1 and 65535', invalidPath: 'Login path must start and end with /', certPair: 'HTTPS requires both certificate and private key paths', needUser: 'Enter a username', needPassword: 'Enter a password', mismatch: 'Passwords do not match', unavailable: 'Port unavailable: {error}'
};
var tr = tapx.translator(ZH, EN);
function tip(value) { return E('span', { 'class': 'cbi-tooltip-container tapx-help' }, [ '?', E('span', { 'class': 'cbi-tooltip' }, value) ]); }
function field(label, control, hint, extraClass) { return E('div', { 'class': 'tapx-field' + (extraClass ? ' ' + extraClass : '') }, [ E('label', { 'class': 'tapx-field-label' }, hint ? [ label, tip(hint) ] : label), E('div', { 'class': 'tapx-field-control' }, control) ]); }
function input(id, type, value, placeholder) { return E('input', { 'id': id, 'class': 'cbi-input-text', 'type': type || 'text', 'value': value || '', 'placeholder': placeholder || '', 'autocomplete': type === 'password' ? 'new-password' : 'off' }); }
function normalizePath(value) { value = String(value || '').trim(); if (!value) return ''; if (value.charAt(0) !== '/') value = '/' + value; if (value.charAt(value.length - 1) !== '/') value += '/'; return value; }
function interfaceSelect(devices, selected) { var options = [ E('option', { 'value': '', 'selected': selected ? null : '' }, tr('all')) ]; devices.forEach(function(device) { var name = device.getName(); options.push(E('option', { 'value': name, 'selected': name === selected ? '' : null }, name)); }); return E('select', { 'id': 'tapx-listen-interface', 'class': 'cbi-input-select' }, options); }

function save(state, trigger) {
	if (trigger && trigger.disabled) return Promise.resolve();
	if (trigger) trigger.disabled = true;
	function unlock() { if (trigger) trigger.disabled = false; }
	var next = {
		interfaceName: document.getElementById('tapx-listen-interface').value,
		port: document.getElementById('tapx-listen-port').value.trim(), basePath: normalizePath(document.getElementById('tapx-base-path').value),
		https: document.getElementById('tapx-panel-https').checked, certFile: document.getElementById('tapx-panel-cert').value.trim(), keyFile: document.getElementById('tapx-panel-key').value.trim(),
		coreAutostart: document.getElementById('tapx-core-autostart').checked, panelAutostart: document.getElementById('tapx-panel-autostart').checked,
		username: document.getElementById('tapx-admin-username').value.trim(), password: document.getElementById('tapx-admin-password').value, confirm: document.getElementById('tapx-admin-confirm').value
	};
	var reset = document.getElementById('tapx-reset-credentials'), needsCredentials = !state.initialized || (reset && reset.value === '1');
	try {
		if (!/^\d+$/.test(next.port) || Number(next.port) < 1 || Number(next.port) > 65535) throw new Error(tr('invalidPort'));
		if (!next.basePath || next.basePath.charAt(0) !== '/' || next.basePath.charAt(next.basePath.length - 1) !== '/') throw new Error(tr('invalidPath'));
		if (next.https && (!next.certFile || !next.keyFile)) throw new Error(tr('certPair'));
		if (needsCredentials && !next.username) throw new Error(tr('needUser'));
		if (needsCredentials && !next.password) throw new Error(tr('needPassword'));
		if (needsCredentials && next.password !== next.confirm) throw new Error(tr('mismatch'));
	} catch (error) { unlock(); tapx.notify(error.message, 'danger'); return Promise.resolve(); }
	var checkArgs = [ '-check-listen', '-listen', ':' + next.port ]; if (next.interfaceName) checkArgs.push('-listen-interface', next.interfaceName);
	var flow = state.panelRunning ? tapx.run(tapx.PANEL_INIT, [ 'stop' ]) : Promise.resolve();
	flow = flow.then(function() {
		return tapx.run(PANEL_BIN, checkArgs).catch(function(error) { throw new Error(tr('unavailable', { error: error.message })); });
	});
	flow = flow.then(function() {
		var args = [ '-db', state.dbPath, '-listen', ':' + next.port, '-base-path', next.basePath ];
		if (next.https) args = args.concat([ '-panel-cert-file', next.certFile, '-panel-key-file', next.keyFile ]); else args.push('-disable-panel-https');
		if (!needsCredentials) return tapx.run(PANEL_BIN, args.concat([ '-set-panel-endpoint' ]));
		return fs.write(PASSWORD_UPLOAD, next.password).then(function() {
			return tapx.run(CONFIG_HELPER, [ 'init-panel', state.dbPath, ':' + next.port, next.basePath, next.username, next.https ? '1' : '0', next.certFile, next.keyFile ]);
		});
	}).then(function() {
		uci.set('tapx', 'panel', 'initialized', '1'); uci.set('tapx', 'panel', 'enabled', '1'); uci.set('tapx', 'panel', 'listen_interface', next.interfaceName);
		uci.set('tapx', 'panel', 'listen_port', next.port); uci.set('tapx', 'panel', 'base_path', next.basePath); uci.set('tapx', 'panel', 'https', next.https ? '1' : '0');
		uci.set('tapx', 'panel', 'cert_file', next.https ? next.certFile : ''); uci.set('tapx', 'panel', 'key_file', next.https ? next.keyFile : ''); uci.set('tapx', 'panel', 'autostart', next.panelAutostart ? '1' : '0');
		uci.set('tapx', 'core', 'autostart', next.coreAutostart ? '1' : '0');
		return tapx.applyChanges();
	}).then(function() { return Promise.all([ tapx.run(tapx.CORE_INIT, [ next.coreAutostart ? 'enable' : 'disable' ]), tapx.run(tapx.PANEL_INIT, [ next.panelAutostart ? 'enable' : 'disable' ]) ]); }).then(function() { return tapx.run(tapx.PANEL_INIT, [ 'restart' ]); }).then(function() { tapx.notify(tr('saved')); window.setTimeout(function() { window.location.reload(); }, 500); }).catch(function(error) {
		fs.remove(PASSWORD_UPLOAD).catch(function() {});
		var recover = state.panelRunning ? tapx.run(tapx.PANEL_INIT, [ 'start' ]).catch(function() {}) : Promise.resolve();
		return recover.then(function() { unlock(); tapx.notify(error.message, 'danger'); });
	});
	return flow;
}

return view.extend({
	load: function() { return Promise.all([ uci.load('tapx'), network.getDevices(), L.resolveDefault(fs.exec(tapx.PIDOF, [ 'tapx-panel' ]), { code: 1 }), L.resolveDefault(fs.exec(tapx.PIDOF, [ 'tapx-core' ]), { code: 1 }) ]); },
	render: function(data) {
		var initialized = uci.get('tapx', 'panel', 'initialized') === '1';
		var panelRunning = data[2].code === 0;
		var coreManaged = initialized && panelRunning;
		var coreRunning = data[3].code === 0 || coreManaged;
		var state = { initialized: initialized, interfaceName: uci.get('tapx', 'panel', 'listen_interface') || '', port: uci.get('tapx', 'panel', 'listen_port') || '', basePath: uci.get('tapx', 'panel', 'base_path') || '', https: uci.get('tapx', 'panel', 'https') === '1', certFile: uci.get('tapx', 'panel', 'cert_file') || '', keyFile: uci.get('tapx', 'panel', 'key_file') || '', coreAutostart: uci.get('tapx', 'core', 'autostart') === '1', panelAutostart: uci.get('tapx', 'panel', 'autostart') === '1', dbPath: uci.get('tapx', 'panel', 'db_path') || '/etc/tapx/tapx.db', panelRunning: panelRunning };
		var httpsToggle = E('input', { 'id': 'tapx-panel-https', 'type': 'checkbox', 'checked': state.https ? '' : null });
		httpsToggle.addEventListener('change', function(event) { document.getElementById('tapx-cert-fields').classList.toggle('hidden', !event.currentTarget.checked); });
		var credentialFields = E('div', { 'id': 'tapx-credential-fields', 'class': state.initialized ? 'hidden' : '' }, [ field(tr('username'), input('tapx-admin-username', 'text', '', 'admin'), tr('userHint')), field(tr('password'), input('tapx-admin-password', 'password', '', '')), field(tr('confirm'), input('tapx-admin-confirm', 'password', '', '')) ]);
		var credentialStatus = state.initialized ? E('div', { 'class': 'tapx-options' }, [ tapx.status(true), E('span', {}, tr('configured')), tapx.button(tr('reset'), 'action', function() { credentialFields.classList.remove('hidden'); document.getElementById('tapx-reset-credentials').value = '1'; }), E('input', { 'id': 'tapx-reset-credentials', 'type': 'hidden', 'value': '0' }) ]) : null;
		var credentialContent = []; if (credentialStatus) credentialContent.push(credentialStatus); credentialContent.push(credentialFields);
		var actions = [ tapx.button(tr('save'), 'apply', function(event) { return save(state, event.currentTarget); }) ];
		if (state.initialized) actions.unshift(E('a', { 'class': 'btn cbi-button cbi-button-action', 'href': tapx.panelUrl(state.port, normalizePath(state.basePath), state.https), 'target': '_blank', 'rel': 'noopener' }, tr('open')));
		return E('div', { 'class': 'tapx-page' }, [ tapx.styles(), E('section', {}, [
			E('div', { 'class': 'tapx-form-group' }, [ E('h3', {}, tr('luci')), E('div', {}, [ field(tr('language'), tapx.languageControl()) ]) ]),
			E('div', { 'class': 'tapx-form-group tapx-runtime-group' }, [ E('h3', {}, tr('runtime')), E('div', { 'class': 'tapx-service-list' }, [ tapx.serviceCard(tapx.tr('core'), 'core', tapx.CORE_INIT, coreRunning, state.coreAutostart, 'tapx-core-autostart', state.initialized, coreManaged), tapx.serviceCard(tapx.tr('panel'), 'panel', tapx.PANEL_INIT, panelRunning, state.panelAutostart, 'tapx-panel-autostart', state.initialized) ]) ]),
			E('div', { 'class': 'tapx-form-group' }, [ E('h3', {}, tr('access')), E('div', {}, [ field(tr('iface'), interfaceSelect(data[1], state.interfaceName), tr('ifaceHint')), field(tr('port'), input('tapx-listen-port', 'number', state.port, '2053'), tr('portHint')), field(tr('path'), input('tapx-base-path', 'text', state.basePath, '/tapx/'), tr('pathHint')), field('HTTPS', E('label', { 'class': 'tapx-switch-line' }, [ httpsToggle ]), null, 'tapx-switch-field'), E('div', { 'id': 'tapx-cert-fields', 'class': state.https ? '' : 'hidden' }, [ field(tr('cert'), input('tapx-panel-cert', 'text', state.certFile, '/etc/ssl/tapx/fullchain.pem'), tr('certHint')), field(tr('key'), input('tapx-panel-key', 'text', state.keyFile, '/etc/ssl/tapx/privkey.pem'), tr('keyHint')) ]) ]) ]),
			E('div', { 'class': 'tapx-form-group' }, [ E('h3', {}, tr('credentials')), E('div', {}, credentialContent) ]),
			E('div', { 'class': 'tapx-page-actions' }, actions)
		]) ]);
	},
	handleSaveApply: null, handleSave: null, handleReset: null
});
