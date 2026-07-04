package model

func (d Device) IsEnabled() bool {
	return d.Enabled
}

func (l Listener) IsEnabled() bool {
	return l.Enabled
}

func (c Connector) IsEnabled() bool {
	return c.Enabled
}

func (c Client) IsEnabled() bool {
	return c.Enabled
}

func (r Route) IsEnabled() bool {
	return r.Enabled
}

func (v VKey) IsEnabled() bool {
	return v.Enabled
}

func (a AddressLimit) IsEnabled() bool {
	return a.Enabled
}

func (x XrayProfile) IsEnabled() bool {
	return x.Enabled
}

func (s Settings) IsEnabled() bool {
	return s.Enabled
}
