'use strict';
'require baseclass';
'require fs';
'require uci';
'require ui';

var LANGUAGE_KEY = 'tapx.luci.language';
var CORE_INIT = '/etc/init.d/tapx';
var PANEL_INIT = '/etc/init.d/tapx-panel';
var PIDOF = '/bin/pidof';

var ZH = {
	overview: '\u6982\u89c8', panelSettings: '\u9ad8\u7ea7\u8bbe\u7f6e', backup: '\u5907\u4efd\u6062\u590d', logs: '\u65e5\u5fd7',
	core: 'TapX \u6838\u5fc3', panel: 'TapX-UI', running: '\u8fd0\u884c\u4e2d', stopped: '\u5df2\u505c\u6b62',
	start: '\u542f\u52a8', restart: '\u91cd\u542f', stop: '\u505c\u6b62', boot: '\u5f00\u673a\u542f\u52a8', operationFailed: '\u64cd\u4f5c\u5931\u8d25'
};
var EN = {
	overview: 'Overview', panelSettings: 'Advanced settings', backup: 'Backup and restore', logs: 'Logs', core: 'TapX Core', panel: 'TapX-UI',
	running: 'Running', stopped: 'Stopped', start: 'Start', restart: 'Restart', stop: 'Stop', boot: 'Start at boot', operationFailed: 'Operation failed'
};

function language() {
	try { var saved = window.localStorage.getItem(LANGUAGE_KEY); if (saved === 'zh' || saved === 'en') return saved; } catch (e) {}
	return String((L.env || {}).lang || navigator.language || '').toLowerCase().indexOf('en') === 0 ? 'en' : 'zh';
}

function tr(key, values) {
	var out = (language() === 'en' ? EN : ZH)[key] || key;
	Object.keys(values || {}).forEach(function(name) { out = out.split('{' + name + '}').join(String(values[name])); });
	return out;
}

function translator(zh, en) {
	return function(key, values) {
		var out = (language() === 'en' ? en : zh)[key] || key;
		Object.keys(values || {}).forEach(function(name) { out = out.split('{' + name + '}').join(String(values[name])); });
		return out;
	};
}

function output(result) {
	return [ result && result.stdout, result && result.stderr ].filter(Boolean).map(function(value) { return value.trim(); }).filter(Boolean).join('\n');
}

function run(path, args) {
	return fs.exec(path, args || []).then(function(result) {
		if (!result || result.code !== 0) throw new Error(output(result) || tr('operationFailed'));
		return result;
	});
}

function notify(message, level) { ui.addNotification(null, E('p', {}, message), level || 'info'); }
function button(label, style, handler) { return E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-' + style, 'click': handler }, label); }
function status(running) { return E('span', { 'class': 'tapx-status ' + (running ? 'is-running' : 'is-stopped') }, [ E('i'), running ? tr('running') : tr('stopped') ]); }
function applyChanges() {
	return uci.save().then(function() { return uci.commit('tapx'); });
}

function languageControl() {
	return E('select', { 'class': 'cbi-input-select',
		'change': function(event) {
			try { window.localStorage.setItem(LANGUAGE_KEY, event.currentTarget.value); } catch (e) {}
			window.location.reload();
		}
	}, [ E('option', { 'value': 'zh', 'selected': language() === 'zh' ? '' : null }, '\u4e2d\u6587'), E('option', { 'value': 'en', 'selected': language() === 'en' ? '' : null }, 'English') ]);
}

function pageHeader(title, subtitle) {
	var heading = [ E('h2', {}, title) ];
	if (subtitle) heading.push(E('p', {}, subtitle));
	return E('div', { 'class': 'tapx-page-header' }, heading);
}

function panelUrl(port, basePath, https) {
	var host = window.location.hostname || '';
	if (host.indexOf(':') >= 0 && host.charAt(0) !== '[') host = '[' + host + ']';
	return (https ? 'https://' : 'http://') + host + ':' + port + basePath;
}

function syncNavigation() {
	var labels = { overview: tr('overview'), panel: tr('panelSettings'), backup: tr('backup'), logs: tr('logs') };
	window.setTimeout(function() {
		document.querySelectorAll('a[href]').forEach(function(anchor) {
			Object.keys(labels).forEach(function(route) {
				if (anchor.pathname && anchor.pathname.slice(-('/admin/services/tapx/' + route).length) === '/admin/services/tapx/' + route) anchor.textContent = labels[route];
			});
		});
	}, 0);
}

function serviceCard(name, section, initPath, running, autostart, checkboxId, ready) {
	function action(command) {
		var previousEnabled = uci.get('tapx', section, 'enabled') || '0';
		var nextEnabled = command === 'stop' ? '0' : '1';
		var enabledChanged = nextEnabled !== previousEnabled;
		if (enabledChanged) uci.set('tapx', section, 'enabled', nextEnabled);
		var persist = enabledChanged ? applyChanges() : Promise.resolve();
		return persist.then(function() { return run(initPath, [ command ]); }).then(function() { window.setTimeout(function() { window.location.reload(); }, 350); }).catch(function(error) {
			if (!enabledChanged) {
				notify(error.message, 'danger');
				return;
			}
			uci.set('tapx', section, 'enabled', previousEnabled);
			return applyChanges().then(function() { notify(error.message, 'danger'); });
		});
	}
	var actions = [ status(running) ];
	if (checkboxId) actions.push(E('label', { 'class': 'tapx-switch-line' }, [
		E('input', { 'id': checkboxId, 'type': 'checkbox', 'checked': autostart ? '' : null }),
		E('span', {}, tr('boot'))
	]));
	var startButton = button(tr('start'), 'apply', function() { return action('start'); });
	var restartButton = button(tr('restart'), 'action', function() { return action('restart'); });
	var stopButton = button(tr('stop'), 'reset', function() { return action('stop'); });
	if (ready === false) {
		startButton.disabled = true;
		restartButton.disabled = true;
	}
	if (!running) stopButton.disabled = true;
	actions = actions.concat([ startButton, restartButton, stopButton ]);
	return E('section', { 'class': 'tapx-card' }, [
		E('div', { 'class': 'tapx-card-heading' }, [ E('h3', {}, name) ]),
		E('div', { 'class': 'tapx-service-actions' }, actions)
	]);
}

function styles() {
	syncNavigation();
	return E('link', { 'rel': 'stylesheet', 'href': L.resource('tapx/tapx.css') });
}

return baseclass.extend({
	CORE_INIT: CORE_INIT, PANEL_INIT: PANEL_INIT, PIDOF: PIDOF, tr: tr, translator: translator, languageControl: languageControl, panelUrl: panelUrl,
	run: run, output: output, notify: notify, button: button, status: status, applyChanges: applyChanges, pageHeader: pageHeader, serviceCard: serviceCard, styles: styles
});
