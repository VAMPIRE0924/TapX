package netguard

import (
	"strings"

	"tapx/internal/config"
)

// Validator checks a complete configuration against the host network
// immediately before it is applied to the runtime.
type Validator func(config.RuntimeConfig) error

// ConflictError reports host networks or interfaces that prevent a runtime
// configuration from being activated. The configuration may still be saved.
type ConflictError struct {
	Problems []string
}

func (e *ConflictError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return "network configuration conflicts"
	}
	return "network configuration conflicts:\n- " + strings.Join(e.Problems, "\n- ")
}
