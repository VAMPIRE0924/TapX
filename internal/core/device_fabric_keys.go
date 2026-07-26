package core

import (
	"fmt"

	"tapx/internal/config"
)

func udpFabricKey(pipe config.RuntimeUDPPipe) string {
	return fmt.Sprintf("udp:%s:%s:%s:%d", pipe.EndpointKind, pipe.EndpointID, pipe.RouteID, pipe.DispatchSocketIndex)
}

func tcpFabricKey(pipe config.RuntimeTCPPipe) string {
	return fmt.Sprintf("tcp:%s:%s:%s:%s:%t", pipe.EndpointKind, pipe.EndpointID, pipe.RouteID, pipe.DispatchPolicyID, pipe.ExternalXrayBridge)
}

func xrayFabricKey(pipe config.RuntimeXrayPipe) string {
	return fmt.Sprintf("xray:%s:%s:%s:%s:%s", pipe.EndpointKind, pipe.EndpointID, pipe.RouteID, pipe.ClientEmail, pipe.HandlerTag)
}
