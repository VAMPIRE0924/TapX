//go:build !linux

package core

import "tapx/internal/config"

func (s *Supervisor) startStandaloneDevices(*config.GeneratedRuntime) error {
	return nil
}
