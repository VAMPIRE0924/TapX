//go:build !linux

package netguard

import "tapx/internal/config"

func ValidateConfig(config.RuntimeConfig) error {
	return nil
}
