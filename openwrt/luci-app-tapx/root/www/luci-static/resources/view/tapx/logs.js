'use strict';
'require view';
'require fs';
'require tapx.common as tapx';

var ZH = { subtitle: '\u67e5\u770b TapX \u6838\u5fc3\u548c\u9762\u677f\u8fd0\u884c\u65e5\u5fd7', level: '\u65e5\u5fd7\u7b49\u7ea7', all: '\u5168\u90e8', error: '\u9519\u8bef', warning: '\u8b66\u544a', info: '\u4fe1\u606f', debug: '\u8c03\u8bd5', refresh: '\u5237\u65b0', empty: '\u6682\u65e0 TapX \u65e5\u5fd7', noMatch: '\u5f53\u524d\u7b49\u7ea7\u6682\u65e0\u65e5\u5fd7' };
var EN = { subtitle: 'View TapX core and panel runtime logs', level: 'Log level', all: 'All', error: 'Error', warning: 'Warning', info: 'Info', debug: 'Debug', refresh: 'Refresh', empty: 'No TapX logs', noMatch: 'No logs at this level' };
var tr = tapx.translator(ZH, EN);

function lineLevel(line) {
	if (/\b(fatal|panic|error|err)\b/i.test(line)) return 'error';
	if (/\b(warning|warn)\b/i.test(line)) return 'warning';
	if (/\bdebug\b/i.test(line)) return 'debug';
	if (/\b(info|notice)\b/i.test(line)) return 'info';
	return 'other';
}

function tapxLines(content) {
	return content ? content.split('\n').filter(function(line) {
		return /\b(?:tapx|tapx-core|tapx-panel)(?:\[\d+\])?:/i.test(line);
	}) : [];
}

return view.extend({
	load: function() { return L.resolveDefault(fs.exec('/sbin/logread', [ '-e', 'tapx' ]), { code: 1, stdout: '' }); },
	render: function(result) {
		var content = tapx.output(result), lines = tapxLines(content);
		var logView = lines.length ? E('pre', { 'class': 'tapx-log' }, lines.join('\n')) : E('div', { 'class': 'tapx-empty' }, tr('empty'));
		function applyFilter(level) {
			if (!lines.length) { logView.textContent = tr('empty'); return; }
			var filtered = level === 'all' ? lines : lines.filter(function(line) { return lineLevel(line) === level; });
			logView.textContent = filtered.length ? filtered.join('\n') : tr('noMatch');
		}
		var levelSelect = E('select', { 'class': 'cbi-input-select', 'change': function(event) {
			applyFilter(event.currentTarget.value);
		} }, [
			E('option', { 'value': 'all' }, tr('all')),
			E('option', { 'value': 'error' }, tr('error')),
			E('option', { 'value': 'warning' }, tr('warning')),
			E('option', { 'value': 'info' }, tr('info')),
			E('option', { 'value': 'debug' }, tr('debug'))
		]);
		return E('div', { 'class': 'tapx-page' }, [ tapx.styles(), tapx.pageHeader(tapx.tr('logs'), tr('subtitle')), E('section', {}, [
			E('div', { 'class': 'tapx-log-toolbar' }, [ E('label', { 'class': 'tapx-log-filter' }, [ E('span', {}, tr('level')), levelSelect ]), tapx.button(tr('refresh'), 'action', function() {
				return fs.exec('/sbin/logread', [ '-e', 'tapx' ]).then(function(next) {
					content = tapx.output(next); lines = tapxLines(content); applyFilter(levelSelect.value);
				}).catch(function(error) { tapx.notify(error.message || String(error), 'danger'); });
			}) ]),
			logView
		]) ]);
	},
	handleSaveApply: null, handleSave: null, handleReset: null
});
