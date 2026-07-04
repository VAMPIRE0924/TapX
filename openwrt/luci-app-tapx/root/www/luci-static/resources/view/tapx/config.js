'use strict';
'require view';
'require form';
'require fs';
'require uci';
'require ui';

function actionOutput(result) {
	var out = '';
	if (result && result.stdout)
		out += result.stdout.trim();
	if (result && result.stderr) {
		if (out)
			out += '\n';
		out += result.stderr.trim();
	}
	return out || _('command completed');
}

function notifyCommand(title, result, style) {
	ui.addNotification(null, E('div', {}, [
		E('strong', {}, title),
		E('pre', { 'class': 'command-output' }, actionOutput(result))
	]), style || 'info');
}

function runCommand(path, args, title) {
	return fs.exec(path, args).then(function(result) {
		if (result.code !== 0) {
			notifyCommand(title, result, 'danger');
			return Promise.reject(new Error('%s failed with exit code %d'.format(title, result.code)));
		}
		notifyCommand(title, result, 'info');
		return result;
	}).catch(function(err) {
		ui.addNotification(null, E('p', {}, err.message || String(err)), 'danger');
		return Promise.reject(err);
	});
}

var objectKinds = [
	[ 'devices', 'Devices', _('Device') ],
	[ 'listeners', 'Listeners', _('Listener') ],
	[ 'connectors', 'Connectors', _('Connector') ],
	[ 'clients', 'Clients', _('Client') ],
	[ 'routes', 'Routes', _('Route') ],
	[ 'vkeys', 'VKeys', _('vKey') ],
	[ 'addresses', 'Addresses', _('Address Limit') ],
	[ 'xrayProfiles', 'XrayProfiles', _('Xray Profile') ],
	[ 'settings', 'Settings', _('Settings') ]
];

function kindMeta(kind) {
	for (var i = 0; i < objectKinds.length; i++) {
		if (objectKinds[i][0] === kind)
			return objectKinds[i];
	}
	return objectKinds[0];
}

function defaultRuntimeJSON() {
	return '{\n  "Devices": [],\n  "Listeners": [],\n  "Connectors": [],\n  "Clients": [],\n  "Routes": [],\n  "VKeys": [],\n  "Addresses": [],\n  "XrayProfiles": [],\n  "Settings": []\n}\n';
}

function safeID(kind) {
	return kind.replace(/[^a-zA-Z0-9]+/g, '-').replace(/^-|-$/g, '') + '-' + Date.now().toString(36);
}

function templateFor(kind, id) {
	var templates = {
		devices: { ID: id, Enabled: true, Name: '', Type: 'tun', IfName: 'tapx%d', MTU: 1500, MSSClamp: 0, IPv4CIDR: '', IPv6CIDR: '', Bridge: null, Routes: [], DNS: null, Remark: '' },
		listeners: { ID: id, Enabled: true, Name: '', BindHost: '0.0.0.0', BindPort: 40000, Transport: 'udp', XrayProfileID: '', RawUDP: { PeerMode: 'learn', FixedPeer: '', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, ReuseAddr: true, ReusePort: false, KeepAliveSecond: 0, Workers: 0, QueueSize: 0, DTLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false, MTU: 0, ReplayWindow: 0 } }, RawTCP: { LengthMode: 'uint16', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, NoDelay: true, KeepAliveSecond: 30, FastOpen: false, ConnectTimeout: 3, ReconnectSecond: 0, Workers: 0, ReadBuffer: 0, WriteBuffer: 0, TLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false } }, Binding: {}, Remark: '' },
		connectors: { ID: id, Enabled: true, Name: '', Remote: '127.0.0.1', Port: 40000, Transport: 'udp', XrayProfileID: '', RawUDP: { PeerMode: 'fixed', FixedPeer: '', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, ReuseAddr: true, ReusePort: false, KeepAliveSecond: 0, Workers: 0, QueueSize: 0, DTLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false, MTU: 0, ReplayWindow: 0 } }, RawTCP: { LengthMode: 'uint16', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, NoDelay: true, KeepAliveSecond: 30, FastOpen: false, ConnectTimeout: 3, ReconnectSecond: 0, Workers: 0, ReadBuffer: 0, WriteBuffer: 0, TLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false } }, Binding: {}, Remark: '' },
		clients: { ID: id, Enabled: true, Name: '', Email: '', ListenerID: '', CredentialType: '', CredentialValue: '', Binding: {}, AddressID: '', ExpiresAt: 0, TrafficCap: 0, TrafficResetAt: 0, TrafficRXOffset: 0, TrafficTXOffset: 0, Remark: '' },
		routes: { ID: id, Enabled: true, VKeyID: '', ListenerID: '', DeviceID: '', ConnectorID: '', ClientID: '', AddressID: '' },
		vkeys: { ID: id, Enabled: true, Name: '', Value: '', Remark: '' },
		addresses: { ID: id, Enabled: true, Name: '', DeviceID: '', ClientID: '', MACs: [], IPv4CIDRs: [], IPv6CIDRs: [], IPv4Gateway: '', IPv6Gateway: '', DNS: [], Routes: [], AllowDefaultRoute: false, Remark: '' },
		xrayProfiles: { ID: id, Enabled: true, Name: '', Runtime: 'embedded', InboundProtocol: '', InboundSettingsJSON: '{}', OutboundProtocol: '', OutboundSettingsJSON: '{}', Network: '', Security: '', StreamSettingsJSON: '{}', SniffingJSON: '', MuxJSON: '', SockoptJSON: '', FallbacksJSON: '', RoutingJSON: '', DNSJSON: '', PolicyJSON: '', AdvancedJSON: '', Remark: '' },
		settings: { ID: id, Enabled: true, Name: 'Default', PanelListen: '127.0.0.1:8080', PanelHTTPS: false, PanelCertFile: '', PanelKeyFile: '', PanelAuthEnabled: false, AdminUsername: 'admin', AdminPasswordHash: '', SessionTTLSecond: 86400, ExternalXrayPath: '', LogLevel: 'info', StatsIntervalSecond: 5, BackupDir: '', DataDir: '', OpenWrtBuildTarget: 'x86-64', AdvancedJSON: '', Remark: '' }
	};
	return JSON.parse(JSON.stringify(templates[kind] || { ID: id, Enabled: true }));
}

function bindingFields() {
	return [
		{ path: 'Binding.RouteID', label: _('Binding Route') },
		{ path: 'Binding.DeviceID', label: _('Binding Device') },
		{ path: 'Binding.ConnectorID', label: _('Binding Connector') },
		{ path: 'Binding.ClientID', label: _('Binding Client') },
		{ path: 'Binding.VKeyID', label: _('Binding vKey') },
		{ path: 'Binding.AddressID', label: _('Binding Address') }
	];
}

function rawUDPFields() {
	return [
		{ path: 'RawUDP.PeerMode', label: _('UDP Peer Mode'), type: 'select', options: [ '', 'any', 'fixed', 'learn' ] },
		{ path: 'RawUDP.FixedPeer', label: _('UDP Fixed Peer') },
		{ path: 'RawUDP.BindInterface', label: _('UDP Bind Interface') },
		{ path: 'RawUDP.BindAddress', label: _('UDP Bind Address') },
		{ path: 'RawUDP.ReceiveBuffer', label: _('UDP Receive Buffer'), type: 'number' },
		{ path: 'RawUDP.SendBuffer', label: _('UDP Send Buffer'), type: 'number' },
		{ path: 'RawUDP.ReuseAddr', label: _('SO_REUSEADDR'), type: 'checkbox' },
		{ path: 'RawUDP.ReusePort', label: _('SO_REUSEPORT'), type: 'checkbox' },
		{ path: 'RawUDP.KeepAliveSecond', label: _('UDP Keepalive Seconds'), type: 'number' },
		{ path: 'RawUDP.Workers', label: _('UDP Workers'), type: 'number' },
		{ path: 'RawUDP.QueueSize', label: _('UDP Queue Size'), type: 'number' },
		{ path: 'RawUDP.DTLS.Enabled', label: _('DTLS Enabled'), type: 'checkbox' },
		{ path: 'RawUDP.DTLS.CertFile', label: _('DTLS Cert File') },
		{ path: 'RawUDP.DTLS.KeyFile', label: _('DTLS Key File') },
		{ path: 'RawUDP.DTLS.CAFile', label: _('DTLS CA File') },
		{ path: 'RawUDP.DTLS.ServerName', label: _('DTLS Server Name') },
		{ path: 'RawUDP.DTLS.ALPN', label: _('DTLS ALPN'), type: 'array' },
		{ path: 'RawUDP.DTLS.MinVersion', label: _('DTLS Min Version'), type: 'select', options: [ '', '1.0', '1.1', '1.2', '1.3' ] },
		{ path: 'RawUDP.DTLS.MaxVersion', label: _('DTLS Max Version'), type: 'select', options: [ '', '1.0', '1.1', '1.2', '1.3' ] },
		{ path: 'RawUDP.DTLS.AllowInsecure', label: _('DTLS Allow Insecure'), type: 'checkbox' },
		{ path: 'RawUDP.DTLS.MTU', label: _('DTLS MTU'), type: 'number' },
		{ path: 'RawUDP.DTLS.ReplayWindow', label: _('DTLS Replay Window'), type: 'number' }
	];
}

function rawTCPFields() {
	return [
		{ path: 'RawTCP.LengthMode', label: _('TCP Length Mode'), type: 'select', options: [ '', 'uint16', 'uint32' ] },
		{ path: 'RawTCP.BindInterface', label: _('TCP Bind Interface') },
		{ path: 'RawTCP.BindAddress', label: _('TCP Bind Address') },
		{ path: 'RawTCP.ReceiveBuffer', label: _('TCP Receive Buffer'), type: 'number' },
		{ path: 'RawTCP.SendBuffer', label: _('TCP Send Buffer'), type: 'number' },
		{ path: 'RawTCP.NoDelay', label: _('TCP_NODELAY'), type: 'checkbox' },
		{ path: 'RawTCP.KeepAliveSecond', label: _('TCP Keepalive Seconds'), type: 'number' },
		{ path: 'RawTCP.FastOpen', label: _('TCP Fast Open'), type: 'checkbox' },
		{ path: 'RawTCP.ConnectTimeout', label: _('Connect Timeout'), type: 'number' },
		{ path: 'RawTCP.ReconnectSecond', label: _('Reconnect Seconds'), type: 'number' },
		{ path: 'RawTCP.Workers', label: _('TCP Workers'), type: 'number' },
		{ path: 'RawTCP.ReadBuffer', label: _('TCP Read Buffer'), type: 'number' },
		{ path: 'RawTCP.WriteBuffer', label: _('TCP Write Buffer'), type: 'number' },
		{ path: 'RawTCP.TLS.Enabled', label: _('TLS Enabled'), type: 'checkbox' },
		{ path: 'RawTCP.TLS.CertFile', label: _('TLS Cert File') },
		{ path: 'RawTCP.TLS.KeyFile', label: _('TLS Key File') },
		{ path: 'RawTCP.TLS.CAFile', label: _('TLS CA File') },
		{ path: 'RawTCP.TLS.ServerName', label: _('TLS Server Name') },
		{ path: 'RawTCP.TLS.ALPN', label: _('TLS ALPN'), type: 'array' },
		{ path: 'RawTCP.TLS.MinVersion', label: _('TLS Min Version'), type: 'select', options: [ '', '1.0', '1.1', '1.2', '1.3' ] },
		{ path: 'RawTCP.TLS.MaxVersion', label: _('TLS Max Version'), type: 'select', options: [ '', '1.0', '1.1', '1.2', '1.3' ] },
		{ path: 'RawTCP.TLS.AllowInsecure', label: _('TLS Allow Insecure'), type: 'checkbox' }
	];
}

function fieldsFor(kind) {
	var common = [
		{ path: 'ID', label: _('ID') },
		{ path: 'Enabled', label: _('Enabled'), type: 'checkbox' },
		{ path: 'Name', label: _('Name') }
	];
	if (kind === 'devices')
		return common.concat([
			{ path: 'Type', label: _('Type'), type: 'select', options: [ 'tun', 'tap' ] },
			{ path: 'IfName', label: _('Interface Name') },
			{ path: 'MTU', label: _('MTU'), type: 'number' },
			{ path: 'MSSClamp', label: _('MSS Clamp'), type: 'number' },
			{ path: 'IPv4CIDR', label: _('IPv4 CIDR') },
			{ path: 'IPv6CIDR', label: _('IPv6 CIDR') },
			{ path: 'Bridge', label: _('Bridge JSON'), type: 'json' },
			{ path: 'Routes', label: _('Static Routes JSON'), type: 'json' },
			{ path: 'DNS', label: _('DNS JSON'), type: 'json' },
			{ path: 'Remark', label: _('Remark'), type: 'textarea' }
		]);
	if (kind === 'listeners')
		return common.concat([
			{ path: 'BindHost', label: _('Bind Host') },
			{ path: 'BindPort', label: _('Bind Port'), type: 'number' },
			{ path: 'Transport', label: _('Transport'), type: 'select', options: [ 'udp', 'tcp', 'xray' ] },
			{ path: 'XrayProfileID', label: _('Xray Profile') }
		], bindingFields(), rawUDPFields(), rawTCPFields(), [ { path: 'Remark', label: _('Remark'), type: 'textarea' } ]);
	if (kind === 'connectors')
		return common.concat([
			{ path: 'Remote', label: _('Remote') },
			{ path: 'Port', label: _('Port'), type: 'number' },
			{ path: 'Transport', label: _('Transport'), type: 'select', options: [ 'udp', 'tcp', 'xray' ] },
			{ path: 'XrayProfileID', label: _('Xray Profile') }
		], bindingFields(), rawUDPFields(), rawTCPFields(), [ { path: 'Remark', label: _('Remark'), type: 'textarea' } ]);
	if (kind === 'clients')
		return common.concat([
			{ path: 'Email', label: _('Email') },
			{ path: 'ListenerID', label: _('Listener') },
			{ path: 'CredentialType', label: _('Credential Type'), type: 'select', options: [ '', 'uuid', 'password', 'vkey' ] },
			{ path: 'CredentialValue', label: _('Credential Value') }
		], bindingFields(), [
			{ path: 'AddressID', label: _('Address Limit') },
			{ path: 'ExpiresAt', label: _('Expires At'), type: 'number' },
			{ path: 'TrafficCap', label: _('Traffic Cap'), type: 'number' },
			{ path: 'TrafficResetAt', label: _('Traffic Reset At'), type: 'number' },
			{ path: 'TrafficRXOffset', label: _('Traffic RX Offset'), type: 'number' },
			{ path: 'TrafficTXOffset', label: _('Traffic TX Offset'), type: 'number' },
			{ path: 'Remark', label: _('Remark'), type: 'textarea' }
		]);
	if (kind === 'routes')
		return [
			{ path: 'ID', label: _('ID') },
			{ path: 'Enabled', label: _('Enabled'), type: 'checkbox' },
			{ path: 'VKeyID', label: _('vKey') },
			{ path: 'ListenerID', label: _('Listener') },
			{ path: 'DeviceID', label: _('Device') },
			{ path: 'ConnectorID', label: _('Connector') },
			{ path: 'ClientID', label: _('Client') },
			{ path: 'AddressID', label: _('Address Limit') }
		];
	if (kind === 'vkeys')
		return common.concat([
			{ path: 'Value', label: _('Value'), type: 'textarea' },
			{ path: 'Remark', label: _('Remark'), type: 'textarea' }
		]);
	if (kind === 'addresses')
		return common.concat([
			{ path: 'DeviceID', label: _('Device') },
			{ path: 'ClientID', label: _('Client') },
			{ path: 'MACs', label: _('MACs'), type: 'array' },
			{ path: 'IPv4CIDRs', label: _('IPv4 CIDRs'), type: 'array' },
			{ path: 'IPv6CIDRs', label: _('IPv6 CIDRs'), type: 'array' },
			{ path: 'IPv4Gateway', label: _('IPv4 Gateway') },
			{ path: 'IPv6Gateway', label: _('IPv6 Gateway') },
			{ path: 'DNS', label: _('DNS'), type: 'array' },
			{ path: 'Routes', label: _('Pushed Routes'), type: 'array' },
			{ path: 'AllowDefaultRoute', label: _('Allow Default Route'), type: 'checkbox' },
			{ path: 'Remark', label: _('Remark'), type: 'textarea' }
		]);
	if (kind === 'xrayProfiles')
		return common.concat([
			{ path: 'Runtime', label: _('Runtime'), type: 'select', options: [ '', 'embedded', 'external' ] },
			{ path: 'InboundProtocol', label: _('Inbound Protocol') },
			{ path: 'InboundSettingsJSON', label: _('Inbound Settings JSON'), type: 'textarea' },
			{ path: 'OutboundProtocol', label: _('Outbound Protocol') },
			{ path: 'OutboundSettingsJSON', label: _('Outbound Settings JSON'), type: 'textarea' },
			{ path: 'Network', label: _('Network') },
			{ path: 'Security', label: _('Security') },
			{ path: 'StreamSettingsJSON', label: _('Stream Settings JSON'), type: 'textarea' },
			{ path: 'SniffingJSON', label: _('Sniffing JSON'), type: 'textarea' },
			{ path: 'MuxJSON', label: _('Mux JSON'), type: 'textarea' },
			{ path: 'SockoptJSON', label: _('Sockopt JSON'), type: 'textarea' },
			{ path: 'FallbacksJSON', label: _('Fallbacks JSON'), type: 'textarea' },
			{ path: 'RoutingJSON', label: _('Routing JSON'), type: 'textarea' },
			{ path: 'DNSJSON', label: _('DNS JSON'), type: 'textarea' },
			{ path: 'PolicyJSON', label: _('Policy JSON'), type: 'textarea' },
			{ path: 'AdvancedJSON', label: _('Advanced JSON'), type: 'textarea' },
			{ path: 'Remark', label: _('Remark'), type: 'textarea' }
		]);
	return common.concat([
		{ path: 'PanelListen', label: _('Panel Listen') },
		{ path: 'PanelHTTPS', label: _('Panel HTTPS'), type: 'checkbox' },
		{ path: 'PanelCertFile', label: _('Panel Cert File') },
		{ path: 'PanelKeyFile', label: _('Panel Key File') },
		{ path: 'PanelAuthEnabled', label: _('Panel Auth Enabled'), type: 'checkbox' },
		{ path: 'AdminUsername', label: _('Admin Username') },
		{ path: 'AdminPasswordHash', label: _('Admin Password Hash'), type: 'textarea' },
		{ path: 'SessionTTLSecond', label: _('Session TTL Seconds'), type: 'number' },
		{ path: 'ExternalXrayPath', label: _('External Xray Path') },
		{ path: 'LogLevel', label: _('Log Level') },
		{ path: 'StatsIntervalSecond', label: _('Stats Interval Seconds'), type: 'number' },
		{ path: 'BackupDir', label: _('Backup Dir') },
		{ path: 'DataDir', label: _('Data Dir') },
		{ path: 'OpenWrtBuildTarget', label: _('OpenWrt Build Target'), type: 'select', options: [ '', 'x86-64' ] },
		{ path: 'AdvancedJSON', label: _('Advanced JSON'), type: 'textarea' },
		{ path: 'Remark', label: _('Remark'), type: 'textarea' }
	]);
}

function fieldGroupsFor(kind) {
	var fields = fieldsFor(kind);

	function byPath(paths) {
		return fields.filter(function(field) {
			return paths.indexOf(field.path) !== -1;
		});
	}

	function byPrefix(prefix) {
		return fields.filter(function(field) {
			return field.path.indexOf(prefix) === 0;
		});
	}

	function rest(used) {
		return fields.filter(function(field) {
			return used.indexOf(field.path) === -1;
		});
	}

	if (kind === 'listeners') {
		var inbound = byPath([ 'ID', 'Enabled', 'Name', 'BindHost', 'BindPort', 'Transport', 'XrayProfileID', 'Remark' ]);
		return [
			{ title: _('Inbound'), fields: inbound },
			{ title: _('Binding'), fields: byPrefix('Binding.') },
			{ title: _('Raw UDP'), fields: byPrefix('RawUDP.') },
			{ title: _('Raw TCP'), fields: byPrefix('RawTCP.') }
		];
	}
	if (kind === 'connectors') {
		var outbound = byPath([ 'ID', 'Enabled', 'Name', 'Remote', 'Port', 'Transport', 'XrayProfileID', 'Remark' ]);
		return [
			{ title: _('Outbound'), fields: outbound },
			{ title: _('Binding'), fields: byPrefix('Binding.') },
			{ title: _('Raw UDP'), fields: byPrefix('RawUDP.') },
			{ title: _('Raw TCP'), fields: byPrefix('RawTCP.') }
		];
	}
	if (kind === 'clients') {
		var client = byPath([ 'ID', 'Enabled', 'Name', 'Email', 'ListenerID', 'CredentialType', 'CredentialValue', 'AddressID', 'ExpiresAt', 'TrafficCap', 'TrafficResetAt', 'TrafficRXOffset', 'TrafficTXOffset', 'Remark' ]);
		return [
			{ title: _('Client'), fields: client },
			{ title: _('Binding'), fields: byPrefix('Binding.') }
		];
	}
	if (kind === 'devices') {
		return [
			{ title: _('Device'), fields: byPath([ 'ID', 'Enabled', 'Name', 'Type', 'IfName', 'MTU', 'MSSClamp', 'IPv4CIDR', 'IPv6CIDR', 'Remark' ]) },
			{ title: _('Bridge'), fields: byPath([ 'Bridge' ]) },
			{ title: _('Routes and DNS'), fields: byPath([ 'Routes', 'DNS' ]) }
		];
	}
	if (kind === 'xrayProfiles') {
		return [
			{ title: _('Xray Profile'), fields: byPath([ 'ID', 'Enabled', 'Name', 'Runtime', 'InboundProtocol', 'OutboundProtocol', 'Network', 'Security', 'Remark' ]) },
			{ title: _('Endpoint JSON'), fields: byPath([ 'InboundSettingsJSON', 'OutboundSettingsJSON' ]) },
			{ title: _('Template JSON'), fields: byPath([ 'StreamSettingsJSON', 'SniffingJSON', 'MuxJSON', 'SockoptJSON', 'FallbacksJSON', 'RoutingJSON', 'DNSJSON', 'PolicyJSON', 'AdvancedJSON' ]) }
		];
	}
	if (kind === 'settings') {
		var panel = byPath([ 'ID', 'Enabled', 'Name', 'PanelListen', 'PanelHTTPS', 'PanelCertFile', 'PanelKeyFile', 'PanelAuthEnabled', 'AdminUsername', 'AdminPasswordHash', 'SessionTTLSecond', 'Remark' ]);
		return [
			{ title: _('Panel'), fields: panel },
			{ title: _('Runtime'), fields: rest(panel.map(function(field) { return field.path; })) }
		];
	}
	return [ { title: kindMeta(kind)[2], fields: fields } ];
}

function getPath(value, path) {
	var parts = path.split('.');
	var current = value;
	for (var i = 0; i < parts.length; i++) {
		if (current == null || typeof current !== 'object')
			return undefined;
		current = current[parts[i]];
	}
	return current;
}

function setPath(value, path, next) {
	var parts = path.split('.');
	var current = value;
	for (var i = 0; i < parts.length - 1; i++) {
		if (current[parts[i]] == null || typeof current[parts[i]] !== 'object')
			current[parts[i]] = {};
		current = current[parts[i]];
	}
	current[parts[parts.length - 1]] = next;
}

function renderObjectBuilder(runtimePath, runtimeJSON) {
	var state = {
		inputs: {},
		kind: 'devices',
		object: templateFor('devices', safeID('devices'))
	};
	var kindSelect = E('select', { 'class': 'cbi-input-select' });
	for (var i = 0; i < objectKinds.length; i++)
		kindSelect.appendChild(E('option', { value: objectKinds[i][0] }, objectKinds[i][2]));
	var fieldsNode = E('div', { 'class': 'cbi-section-node' });
	var objectJSON = E('textarea', { 'class': 'cbi-input-textarea', rows: 12, spellcheck: 'false' });

	function parseRuntime() {
		return JSON.parse(runtimeJSON || defaultRuntimeJSON());
	}

	function currentObjectFromInputs() {
		var obj = templateFor(state.kind, safeID(state.kind));
		var defs = fieldsFor(state.kind);
		for (var i = 0; i < defs.length; i++) {
			var def = defs[i];
			var input = state.inputs[def.path];
			var value = '';
			if (!input)
				continue;
			if (def.type === 'checkbox') {
				value = input.checked;
			} else if (def.type === 'number') {
				value = input.value === '' ? 0 : Number(input.value);
			} else if (def.type === 'json') {
				value = input.value.trim() === '' ? null : JSON.parse(input.value);
			} else if (def.type === 'array') {
				value = input.value.split(/[,\n]/).map(function(item) { return item.trim(); }).filter(function(item) { return item !== ''; });
			} else {
				value = input.value;
			}
			setPath(obj, def.path, value);
		}
		return obj;
	}

	function fillFields(obj) {
		state.inputs = {};
		fieldsNode.innerHTML = '';
		var groups = fieldGroupsFor(state.kind);
		for (var g = 0; g < groups.length; g++) {
			fieldsNode.appendChild(E('h4', {}, groups[g].title));
			for (var i = 0; i < groups[g].fields.length; i++) {
				var def = groups[g].fields[i];
				var value = getPath(obj, def.path);
				var input;
				if (def.type === 'select') {
					input = E('select', { 'class': 'cbi-input-select' });
					for (var j = 0; j < def.options.length; j++)
						input.appendChild(E('option', { value: def.options[j] }, def.options[j]));
					input.value = value == null ? '' : String(value);
				} else if (def.type === 'checkbox') {
					input = E('input', { type: 'checkbox', 'class': 'cbi-input-checkbox' });
					input.checked = Boolean(value);
				} else if (def.type === 'textarea' || def.type === 'json' || def.type === 'array') {
					input = E('textarea', { 'class': 'cbi-input-textarea', rows: def.type === 'array' ? 3 : 5, spellcheck: 'false' });
					if (def.type === 'json')
						input.value = value == null ? '' : JSON.stringify(value, null, 2);
					else if (def.type === 'array')
						input.value = Array.isArray(value) ? value.join('\n') : '';
					else
						input.value = value == null ? '' : String(value);
				} else {
					input = E('input', { type: def.type === 'number' ? 'number' : 'text', 'class': 'cbi-input-text' });
					input.value = value == null ? '' : String(value);
				}
				state.inputs[def.path] = input;
				fieldsNode.appendChild(E('div', { 'class': 'cbi-value' }, [
					E('label', { 'class': 'cbi-value-title' }, def.label),
					E('div', { 'class': 'cbi-value-field' }, input)
				]));
			}
		}
		objectJSON.value = JSON.stringify(obj, null, 2);
	}

	function refreshKind() {
		state.kind = kindSelect.value;
		state.object = templateFor(state.kind, safeID(state.kind));
		fillFields(state.object);
	}

	function updateJSONFromFields() {
		try {
			state.object = currentObjectFromInputs();
			objectJSON.value = JSON.stringify(state.object, null, 2);
		} catch (err) {
			ui.addNotification(null, E('p', {}, err.message || String(err)), 'danger');
		}
	}

	function writeObject() {
		var meta = kindMeta(state.kind);
		var cfg, obj, arr, replaced;
		try {
			cfg = parseRuntime();
			obj = JSON.parse(objectJSON.value);
		} catch (err) {
			ui.addNotification(null, E('p', {}, err.message || String(err)), 'danger');
			return Promise.reject(err);
		}
		if (!obj.ID) {
			ui.addNotification(null, E('p', {}, _('Object ID is required')), 'danger');
			return Promise.reject(new Error('Object ID is required'));
		}
		arr = cfg[meta[1]] || [];
		replaced = false;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i].ID === obj.ID) {
				arr[i] = obj;
				replaced = true;
				break;
			}
		}
		if (!replaced)
			arr.push(obj);
		cfg[meta[1]] = arr;
		runtimeJSON = JSON.stringify(cfg, null, 2) + '\n';
		return fs.write(runtimePath, runtimeJSON).then(function() {
			ui.addNotification(null, E('p', {}, _('Object saved to runtime config')), 'info');
			return runCommand('/usr/bin/tapx-core', [ '-config', runtimePath, '-check' ], _('TapX config check'));
		});
	}

	kindSelect.addEventListener('change', refreshKind);
	fieldsNode.addEventListener('input', updateJSONFromFields);
	fieldsNode.addEventListener('change', updateJSONFromFields);
	refreshKind();

	return E('div', { 'class': 'cbi-section' }, [
		E('h3', {}, _('Object field editor')),
		E('div', { 'class': 'cbi-value' }, [
			E('label', { 'class': 'cbi-value-title' }, _('Object kind')),
			E('div', { 'class': 'cbi-value-field' }, kindSelect)
		]),
		fieldsNode,
		E('div', { 'class': 'cbi-value' }, [
			E('label', { 'class': 'cbi-value-title' }, _('Object JSON')),
			E('div', { 'class': 'cbi-value-field' }, objectJSON)
		]),
		E('div', { 'class': 'cbi-page-actions' }, [
			E('button', {
				'type': 'button',
				'class': 'btn cbi-button cbi-button-action',
				'click': updateJSONFromFields
			}, _('Generate JSON')),
			' ',
			E('button', {
				'type': 'button',
				'class': 'btn cbi-button cbi-button-save',
				'click': writeObject
			}, _('Append / Replace Object'))
		])
	]);
}

return view.extend({
	load: function() {
		return uci.load('tapx').then(function() {
			var configPath = uci.get('tapx', 'core', 'config_path') || '/etc/tapx/runtime.json';
			return Promise.all([
				L.resolveDefault(fs.read(configPath), ''),
				L.resolveDefault(fs.read('/etc/tapx/runtime.json.example'), ''),
				Promise.resolve(configPath)
			]);
		});
	},

	render: function(data) {
		var m, s, o;
		var runtimePath = data[2] || '/etc/tapx/runtime.json';
		var runtimeJSON = data[0] || data[1] || defaultRuntimeJSON();

		m = new form.Map('tapx', _('TapX'));

		s = m.section(form.TypedSection, 'tapx', _('Core service'));
		s.anonymous = true;

		o = s.option(form.Flag, 'enabled', _('Enable'));
		o.default = '0';
		o.rmempty = false;

		o = s.option(form.Value, 'config_path', _('Runtime config path'));
		o.default = '/etc/tapx/runtime.json';
		o.rmempty = false;

		o = s.option(form.Value, 'respawn_threshold', _('Respawn threshold'));
		o.datatype = 'uinteger';
		o.default = '3600';

		o = s.option(form.Value, 'respawn_timeout', _('Respawn timeout'));
		o.datatype = 'uinteger';
		o.default = '5';

		o = s.option(form.Value, 'respawn_retry', _('Respawn retry'));
		o.datatype = 'uinteger';
		o.default = '5';

		s = m.section(form.NamedSection, 'runtime', 'tapx_runtime', _('Runtime JSON'));
		s.addremove = false;

		o = s.option(form.DummyValue, '_runtime_path', _('Loaded runtime path'));
		o.cfgvalue = function() {
			return runtimePath;
		};

		o = s.option(form.Button, '_check_runtime', _('Check saved runtime'));
		o.inputstyle = 'action';
		o.onclick = function() {
			return runCommand('/usr/bin/tapx-core', [ '-config', runtimePath, '-check' ], _('TapX config check'));
		};

		o = s.option(form.Button, '_service_status', _('Service status'));
		o.inputstyle = 'action';
		o.onclick = function() {
			return runCommand('/etc/init.d/tapx', [ 'status' ], _('TapX service status'));
		};

		o = s.option(form.Button, '_reload_service', _('Reload TapX'));
		o.inputstyle = 'apply';
		o.onclick = function() {
			return runCommand('/etc/init.d/tapx', [ 'reload' ], _('TapX service reload'));
		};

		o = s.option(form.TextValue, '_runtime_json', _('Runtime config'));
		o.rows = 24;
		o.monospace = true;
		o.cfgvalue = function() {
			return runtimeJSON;
		};
		o.write = function(sectionID, value) {
			try {
				JSON.parse(value);
			} catch (err) {
				ui.addNotification(null, E('p', {}, _('Runtime JSON is invalid: %s').format(err.message)), 'danger');
				return Promise.reject(err);
			}
			return fs.write(runtimePath, value.trim() + '\n');
		};

		return m.render().then(function(node) {
			node.appendChild(renderObjectBuilder(runtimePath, runtimeJSON));
			return node;
		});
	}
});
