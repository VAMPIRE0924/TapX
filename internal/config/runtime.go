package config

import "tapx/internal/model"

// RuntimeConfig is the control-plane object set loaded from DB/API state.
// GenerateRuntime turns it into the smaller configuration used by tapx-core.
type RuntimeConfig struct {
	Devices      []model.Device
	Listeners    []model.Listener
	Connectors   []model.Connector
	Clients      []model.Client
	Routes       []model.Route
	VKeys        []model.VKey
	Addresses    []model.AddressLimit
	XrayProfiles []model.XrayProfile
	Settings     []model.Settings
}
