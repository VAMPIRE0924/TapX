'use strict';
'require view';
'require fs';
'require network';
'require uci';
'require ui';

var PANEL_INIT = '/etc/init.d/tapx-panel';
var CORE_INIT = '/etc/init.d/tapx';
var PANEL_BIN = '/usr/bin/tapx-panel';
var CONFIG_HELPER = '/usr/libexec/tapx-openwrt-config';
var RESTORE_UPLOAD = '/tmp/tapx-openwrt-restore.tar.gz';

function outputText(result) {
	var lines = [];
	if (result && result.stdout)
		lines.push(result.stdout.trim());
	if (result && result.stderr)
		lines.push(result.stderr.trim());
	return lines.filter(Boolean).join('\n');
}

function notify(message, level) {
	ui.addNotification(null, E('p', {}, message), level || 'info');
}

function run(path, args) {
	return fs.exec(path, args || []).then(function(result) {
		if (!result || result.code !== 0)
			throw new Error(outputText(result) || '操作失败');
		return result;
	});
}

function button(label, style, handler) {
	return E('button', {
		'type': 'button',
		'class': 'btn cbi-button cbi-button-' + style,
		'click': handler
	}, label);
}

function statusLabel(running) {
	return E('span', {
		'class': running ? 'label notice' : 'label warning'
	}, running ? '运行中' : '已停止');
}

function serviceRow(label, section, initPath, running, autostart) {
	function serviceAction(action, message) {
		var prepare = Promise.resolve();
		if (action === 'start' || action === 'restart') {
			uci.set('tapx', section, 'enabled', '1');
			prepare = uci.save();
		}
		return prepare.then(function() {
			return run(initPath, [ action ]);
		}).then(function() {
			notify(message);
			window.setTimeout(function() { window.location.reload(); }, 500);
		}).catch(function(error) {
			notify(error.message, 'danger');
		});
	}
	return E('div', { 'class': 'tapx-service-row' }, [
		E('strong', {}, label),
		statusLabel(running),
		E('div', { 'class': 'tapx-service-actions' }, [
			E('label', { 'class': 'tapx-autostart' }, [
				E('input', {
					'type': 'checkbox',
					'checked': autostart ? '' : null,
					'change': function(event) {
						var enabled = event.currentTarget.checked;
						uci.set('tapx', section, 'autostart', enabled ? '1' : '0');
						if (enabled)
							uci.set('tapx', section, 'enabled', '1');
						return uci.save().then(function() {
							return run(initPath, [ enabled ? 'enable' : 'disable' ]);
						}).then(function() {
							notify(label + (enabled ? '已启用开机启动' : '已关闭开机启动'));
						}).catch(function(error) {
							event.currentTarget.checked = !enabled;
							notify(error.message, 'danger');
						});
					}
				}),
				E('span', {}, '开机启动')
			]),
			button('启动', 'apply', function() { return serviceAction('start', label + '已启动'); }),
			button('重启', 'reload', function() { return serviceAction('restart', label + '已重启'); }),
			button('关闭', 'reset', function() { return serviceAction('stop', label + '已关闭'); })
		])
	]);
}

function normalizePath(value) {
	value = String(value || '').trim();
	if (!value)
		return '';
	if (value.charAt(0) !== '/')
		value = '/' + value;
	if (value.charAt(value.length - 1) !== '/')
		value += '/';
	return value;
}

function panelURL(port, basePath) {
	var host = window.location.hostname;
	if (host.indexOf(':') >= 0)
		host = '[' + host.replace(/^\[|\]$/g, '') + ']';
	return 'http://' + host + ':' + port + normalizePath(basePath);
}

function bytesToBase64(bytes) {
	var binary = '';
	for (var i = 0; i < bytes.length; i++)
		binary += String.fromCharCode(bytes[i]);
	return window.btoa(binary).replace(/=+$/, '');
}

function hashPassword(password) {
	var salt = window.crypto.getRandomValues(new Uint8Array(16));
	return window.crypto.subtle.importKey(
		'raw', new TextEncoder().encode(password), { name: 'PBKDF2' }, false, [ 'deriveBits' ]
	).then(function(key) {
		return window.crypto.subtle.deriveBits({
			name: 'PBKDF2',
			hash: 'SHA-256',
			salt: salt,
			iterations: 120000
		}, key, 256);
	}).then(function(bits) {
		return 'pbkdf2-sha256$120000$' + bytesToBase64(salt) + '$' + bytesToBase64(new Uint8Array(bits));
	});
}

function valueRow(label, control, hint) {
	var title = [ label ];
	if (hint)
		title.push(E('span', { 'class': 'cbi-tooltip-container tapx-help' }, [
			'?',
			E('span', { 'class': 'cbi-tooltip' }, hint)
		]));
	return E('div', { 'class': 'cbi-value' }, [
		E('label', { 'class': 'cbi-value-title' }, title),
		E('div', { 'class': 'cbi-value-field' }, control)
	]);
}

function input(id, type, value, placeholder) {
	return E('input', {
		'id': id,
		'class': 'cbi-input-text',
		'type': type || 'text',
		'value': value || '',
		'placeholder': placeholder || '',
		'autocomplete': type === 'password' ? 'new-password' : 'off'
	});
}

function interfaceSelect(devices, selected) {
	var options = [ E('option', { 'value': '' }, '请选择网卡') ];
	devices.forEach(function(device) {
		var name = device.getName();
		options.push(E('option', {
			'value': name,
			'selected': name === selected ? '' : null
		}, name));
	});
	return E('select', { 'id': 'tapx-listen-interface', 'class': 'cbi-input-select' }, options);
}

function credentialsBlock(initialized) {
	var fields = E('div', {
		'id': 'tapx-credential-fields',
		'class': initialized ? 'hidden' : ''
	}, [
		valueRow('用户名', input('tapx-admin-username', 'text', '', '例如：admin'), '用于登录完整 TapX-UI。保存后此处不回显。'),
		valueRow('密码', input('tapx-admin-password', 'password', '', '请输入新密码'), '仅在浏览器内生成哈希，明文不会写入 UCI 或命令行。'),
		valueRow('确认密码', input('tapx-admin-confirm', 'password', '', '再次输入新密码'))
	]);
	if (!initialized)
		return fields;
	return E('div', {}, [
		E('div', { 'class': 'tapx-credential-status' }, [
			statusLabel(true),
			E('span', {}, '登录凭据已设置且不回显'),
			button('重新设置', 'action', function() {
				fields.classList.remove('hidden');
				document.getElementById('tapx-reset-credentials').value = '1';
			})
		]),
		E('input', { 'id': 'tapx-reset-credentials', 'type': 'hidden', 'value': '0' }),
		fields
	]);
}

function validateSetup(values, needsCredentials) {
	if (!values.interfaceName)
		throw new Error('请选择面板监听网卡');
	if (!/^\d+$/.test(values.port) || Number(values.port) < 1 || Number(values.port) > 65535)
		throw new Error('面板端口必须为 1 至 65535');
	if (!values.basePath || values.basePath.charAt(0) !== '/' || values.basePath.charAt(values.basePath.length - 1) !== '/')
		throw new Error('登录入口必须以 / 开头并以 / 结尾');
	if (needsCredentials) {
		if (!values.username.trim())
			throw new Error('请输入用户名');
		if (!values.password)
			throw new Error('请输入密码');
		if (values.password !== values.confirm)
			throw new Error('两次输入的密码不一致');
	}
}

function readSetupValues() {
	return {
		interfaceName: document.getElementById('tapx-listen-interface').value,
		port: document.getElementById('tapx-listen-port').value.trim(),
		basePath: normalizePath(document.getElementById('tapx-base-path').value),
		autostart: document.getElementById('tapx-autostart').checked,
		startNow: document.getElementById('tapx-start-now').checked,
		username: (document.getElementById('tapx-admin-username') || {}).value || '',
		password: (document.getElementById('tapx-admin-password') || {}).value || '',
		confirm: (document.getElementById('tapx-admin-confirm') || {}).value || ''
	};
}

function saveSetup(state) {
	var values = readSetupValues();
	var reset = document.getElementById('tapx-reset-credentials');
	var needsCredentials = !state.initialized || (reset && reset.value === '1');
	validateSetup(values, needsCredentials);

	var changedSocket = state.interfaceName !== values.interfaceName || state.port !== values.port;
	var stoppedForCheck = false;
	var flow = Promise.resolve();
	if (state.panelRunning && changedSocket) {
		flow = flow.then(function() {
			stoppedForCheck = true;
			return run(PANEL_INIT, [ 'stop' ]);
		});
	}
	flow = flow.then(function() {
		if (state.panelRunning && !changedSocket)
			return null;
		return run(PANEL_BIN, [
			'-check-listen',
			'-listen', ':' + values.port,
			'-listen-interface', values.interfaceName
		]);
	}).catch(function(error) {
		if (stoppedForCheck)
			run(PANEL_INIT, [ 'start' ]);
		throw new Error('端口不可用：' + error.message);
	});

	flow = flow.then(function() {
		if (!needsCredentials)
			return run(PANEL_BIN, [
				'-db', state.dbPath,
				'-listen', ':' + values.port,
				'-base-path', values.basePath,
				'-set-panel-endpoint'
			]);
		return hashPassword(values.password).then(function(hash) {
			return run(PANEL_BIN, [
				'-db', state.dbPath,
				'-listen', ':' + values.port,
				'-base-path', values.basePath,
				'-init-admin',
				'-admin-username', values.username.trim(),
				'-admin-password-hash', hash
			]);
		});
	});

	return flow.then(function() {
		uci.set('tapx', 'panel', 'enabled', '1');
		uci.set('tapx', 'panel', 'initialized', '1');
		uci.set('tapx', 'panel', 'listen_interface', values.interfaceName);
		uci.set('tapx', 'panel', 'listen_port', values.port);
		uci.set('tapx', 'panel', 'base_path', values.basePath);
		uci.set('tapx', 'panel', 'autostart', values.autostart ? '1' : '0');
		return uci.save();
	}).then(function() {
		return run(PANEL_INIT, [ values.autostart ? 'enable' : 'disable' ]);
	}).then(function() {
		if (values.startNow)
			return run(PANEL_INIT, [ 'restart' ]);
		if (stoppedForCheck)
			return null;
		return null;
	}).then(function() {
		notify('TapX-UI 设置已保存');
		window.setTimeout(function() { window.location.reload(); }, 700);
	});
}

function setupSection(state, devices) {
	var saveButton = button(state.initialized ? '保存设置' : '初始化并保存', 'apply', function(event) {
		var target = event.currentTarget;
		target.disabled = true;
		return saveSetup(state).catch(function(error) {
			notify(error.message, 'danger');
			target.disabled = false;
		});
	});
	return E('div', { 'class': 'cbi-section' }, [
		E('h3', {}, 'TapX-UI 基本设置'),
		valueRow('监听网卡', interfaceSelect(devices, state.interfaceName), '只接受从所选系统网卡进入的面板连接。'),
		valueRow('面板端口', input('tapx-listen-port', 'number', state.port, '例如：2053'), '保存时检查端口是否可用。'),
		valueRow('登录入口', input('tapx-base-path', 'text', state.basePath, '例如：/tapx/'), '必须以 / 开头并以 / 结尾。'),
		valueRow('开机启动', E('input', {
			'id': 'tapx-autostart', 'type': 'checkbox', 'checked': state.autostart ? '' : null
		}), '随 OpenWrt 启动 TapX-UI。'),
		valueRow('保存后启动', E('input', {
			'id': 'tapx-start-now', 'type': 'checkbox', 'checked': ''
		})),
		E('h4', {}, '登录凭据'),
		credentialsBlock(state.initialized),
		E('div', { 'class': 'cbi-page-actions' }, [
			saveButton,
			state.initialized ? button('打开 TapX-UI', 'action', function() {
				window.open(panelURL(state.port, state.basePath), '_blank', 'noopener');
			}) : ''
		])
	]);
}

function downloadConfig() {
	return fs.exec_direct(CONFIG_HELPER, [ 'export' ], 'blob').then(function(blob) {
		var url = window.URL.createObjectURL(blob);
		var stamp = new Date().toISOString().replace(/[-:]/g, '').replace(/\..*$/, 'Z');
		var anchor = E('a', {
			'href': url,
			'download': 'tapx-openwrt-' + stamp + '.tar.gz'
		});
		document.body.appendChild(anchor);
		anchor.click();
		anchor.remove();
		window.setTimeout(function() { window.URL.revokeObjectURL(url); }, 1000);
	}).catch(function(error) {
		notify(error.message || String(error), 'danger');
	});
}

function restoreConfig() {
	return ui.uploadFile(RESTORE_UPLOAD, null, '仅接受由 TapX OpenWrt 导出的配置包。').then(function() {
		if (!window.confirm('恢复会替换当前 TapX 数据库和面板设置，是否继续？'))
			return fs.remove(RESTORE_UPLOAD);
		return run(CONFIG_HELPER, [ 'restore', RESTORE_UPLOAD ]).then(function() {
			notify('TapX 配置已恢复');
			window.setTimeout(function() { window.location.reload(); }, 700);
		});
	}).catch(function(error) {
		notify(error.message || String(error), 'danger');
	});
}

function resetConfig() {
	if (!window.confirm('恢复固件内置的 TapX 配置和数据库？'))
		return Promise.resolve();
	return run(CONFIG_HELPER, [ 'reset' ]).then(function() {
		notify('已恢复固件内置配置');
		window.setTimeout(function() { window.location.reload(); }, 700);
	}).catch(function(error) {
		notify(error.message || String(error), 'danger');
	});
}

function backupSection(initialized) {
	return E('div', { 'class': 'cbi-section' }, [
		E('h3', {}, '备份与恢复'),
		E('div', { 'class': 'tapx-backup-row' }, [
			E('span', {}, '仅导出 TapX 数据库和面板设置，不包含证书。'),
			E('div', { 'class': 'tapx-service-actions' }, [
				button('导出配置', 'action', initialized ? downloadConfig : function() {
					notify('请先完成 TapX-UI 初始化', 'warning');
				}),
				button('恢复配置', 'reload', restoreConfig),
				button('重置到固件', 'reset', resetConfig)
			])
		])
	]);
}

function styles() {
	return E('style', {}, [
		'.tapx-service-row{display:grid;grid-template-columns:minmax(120px,1fr) auto minmax(260px,2fr);gap:12px;align-items:center;padding:12px 0;border-bottom:1px solid var(--border-color-medium,#ddd)}',
		'.tapx-service-actions{display:flex;justify-content:flex-end;gap:8px;flex-wrap:wrap}',
		'.tapx-autostart{display:inline-flex;align-items:center;gap:6px;margin-right:6px}',
		'.tapx-credential-status{display:flex;align-items:center;gap:10px;padding:8px 0 14px}',
		'.tapx-help{display:inline-flex;align-items:center;justify-content:center;width:16px;height:16px;margin-left:6px;border:1px solid currentColor;border-radius:50%;font-size:11px;cursor:help}',
		'.tapx-log{max-height:360px;overflow:auto;white-space:pre-wrap}',
		'.tapx-backup-row{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;padding:8px 0}',
		'.hidden{display:none!important}',
		'@media(max-width:700px){.tapx-service-row{grid-template-columns:1fr auto}.tapx-service-actions{grid-column:1/-1;justify-content:flex-start}}'
	]);
}

return view.extend({
	load: function() {
		return Promise.all([
			uci.load('tapx'),
			network.getDevices(),
			L.resolveDefault(fs.exec(CORE_INIT, [ 'status' ]), { code: 1 }),
			L.resolveDefault(fs.exec(PANEL_INIT, [ 'status' ]), { code: 1 }),
			L.resolveDefault(fs.exec(CORE_INIT, [ 'enabled' ]), { code: 1 }),
			L.resolveDefault(fs.exec(PANEL_INIT, [ 'enabled' ]), { code: 1 }),
			L.resolveDefault(fs.exec('/sbin/logread', [ '-e', 'tapx' ]), { code: 1, stdout: '' })
		]);
	},

	render: function(data) {
		var state = {
			initialized: uci.get('tapx', 'panel', 'initialized') === '1',
			interfaceName: uci.get('tapx', 'panel', 'listen_interface') || '',
			port: uci.get('tapx', 'panel', 'listen_port') || '',
			basePath: uci.get('tapx', 'panel', 'base_path') || '',
			autostart: uci.get('tapx', 'panel', 'autostart') === '1',
			dbPath: uci.get('tapx', 'panel', 'db_path') || '/etc/tapx/tapx.db',
			panelRunning: data[3] && data[3].code === 0
		};
		return E('div', {}, [
			styles(),
			E('h2', {}, 'TapX'),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, '服务'),
				serviceRow('TapX 核心', 'core', CORE_INIT, data[2] && data[2].code === 0, data[4] && data[4].code === 0),
				serviceRow('TapX-UI', 'panel', PANEL_INIT, state.panelRunning, data[5] && data[5].code === 0)
			]),
			setupSection(state, data[1]),
			backupSection(state.initialized),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, '日志'),
				E('pre', { 'class': 'tapx-log' }, outputText(data[6]) || '暂无日志'),
				E('div', { 'class': 'cbi-page-actions' }, [
					button('刷新', 'action', function() { window.location.reload(); })
				])
			])
		]);
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
