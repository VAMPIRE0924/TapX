package config

import (
	"fmt"

	"tapx/internal/pathmtu"
)

// CompileConfirmedUDPPath writes one peer-confirmed immutable plan into the
// generated runtime copy. The persisted operator configuration remains an
// upper-bound source of truth and is never rewritten by path discovery.
func CompileConfirmedUDPPath(runtime *GeneratedRuntime, confirmation pathmtu.ConfirmedPath) error {
	if runtime == nil {
		return fmt.Errorf("runtime is required")
	}
	if confirmation.Key.EndpointKind == "" || confirmation.Key.EndpointID == "" || confirmation.Key.DeviceID == "" {
		return fmt.Errorf("confirmed path key is incomplete")
	}
	if confirmation.Plan.PathMTU <= 0 || confirmation.Plan.MaxWirePayload <= pathmtu.SegmentHeaderSize ||
		confirmation.Plan.MaxFrameSize <= 0 || confirmation.Plan.MaxSegmentFrameSize <= 0 {
		return fmt.Errorf("confirmed path plan is incomplete")
	}
	for index := range runtime.UDPPipes {
		pipe := &runtime.UDPPipes[index]
		if pipe.EndpointKind != confirmation.Key.EndpointKind || pipe.EndpointID != confirmation.Key.EndpointID {
			continue
		}
		if pipe.DeviceID != confirmation.Key.DeviceID {
			return fmt.Errorf("confirmed path device %s does not match runtime device %s", confirmation.Key.DeviceID, pipe.DeviceID)
		}
		if !pipe.LinkAutoOptimize {
			return fmt.Errorf("UDP pipe %s does not enable automatic link optimization", pipe.EndpointID)
		}
		if confirmation.Plan.MaxFrameSize != pipe.MaxFrameSize {
			return fmt.Errorf("confirmed frame ceiling %d does not match runtime ceiling %d", confirmation.Plan.MaxFrameSize, pipe.MaxFrameSize)
		}
		pipe.MaxDatagramPayload = confirmation.Plan.MaxWirePayload
		pipe.ConfirmedPathMTU = confirmation.Plan.PathMTU
		pipe.EffectiveNetworkMTU = confirmation.Plan.EffectiveNetworkMTU
		pipe.TCPMSSIPv4 = confirmation.Plan.TCPMSSIPv4
		pipe.TCPMSSIPv6 = confirmation.Plan.TCPMSSIPv6
		return nil
	}
	return fmt.Errorf("confirmed path references missing UDP pipe %s/%s", confirmation.Key.EndpointKind, confirmation.Key.EndpointID)
}
