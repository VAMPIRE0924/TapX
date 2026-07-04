package core

import (
	"fmt"

	"tapx/internal/config"
	"tapx/internal/xrayruntime"
)

type Supervisor struct {
	udpPipes  []*UDPPipeHandle
	tcpPipes  []*TCPPipeHandle
	xrayPipes []*XrayPipeHandle
	xray      *xrayruntime.Manager
}

func NewSupervisor() *Supervisor {
	return &Supervisor{}
}

func (s *Supervisor) Start(runtime *config.GeneratedRuntime) error {
	if runtime == nil {
		return fmt.Errorf("core: runtime is nil")
	}
	if len(s.udpPipes) != 0 || len(s.tcpPipes) != 0 || len(s.xrayPipes) != 0 || s.xray != nil {
		return fmt.Errorf("core: supervisor already started")
	}
	for _, pipe := range runtime.UDPPipes {
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			_ = s.Stop()
			return fmt.Errorf("core: udp pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startUDPPipe(pipe, device)
		if err != nil {
			_ = s.Stop()
			return err
		}
		s.udpPipes = append(s.udpPipes, handle)
	}
	for _, pipe := range runtime.TCPPipes {
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			_ = s.Stop()
			return fmt.Errorf("core: tcp pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startTCPPipe(pipe, device)
		if err != nil {
			_ = s.Stop()
			return err
		}
		s.tcpPipes = append(s.tcpPipes, handle)
	}
	xray := xrayruntime.NewManager()
	for _, pipe := range runtime.XrayPipes {
		if pipe.EndpointKind != "listener" {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			_ = s.Stop()
			return fmt.Errorf("core: xray pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startXrayPipe(pipe, device, xray)
		if err != nil {
			_ = s.Stop()
			return err
		}
		s.xrayPipes = append(s.xrayPipes, handle)
	}
	if err := xray.Start(runtime); err != nil {
		_ = xray.Stop()
		_ = s.Stop()
		return err
	}
	if xray.State().EndpointCount > 0 {
		s.xray = xray
	}
	for _, pipe := range runtime.XrayPipes {
		if pipe.EndpointKind != "connector" {
			continue
		}
		device, ok := findRuntimeDevice(runtime.Devices, pipe.DeviceID)
		if !ok {
			_ = s.Stop()
			return fmt.Errorf("core: xray pipe %s references missing device %s", pipe.EndpointID, pipe.DeviceID)
		}
		handle, err := startXrayPipe(pipe, device, xray)
		if err != nil {
			_ = s.Stop()
			return err
		}
		s.xrayPipes = append(s.xrayPipes, handle)
	}
	return nil
}

func (s *Supervisor) Stop() error {
	var firstErr error
	if s.xray != nil {
		if err := s.xray.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.xray = nil
	}
	for i := len(s.xrayPipes) - 1; i >= 0; i-- {
		if err := s.xrayPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.xrayPipes = nil
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		if err := s.tcpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.tcpPipes = nil
	for i := len(s.udpPipes) - 1; i >= 0; i-- {
		if err := s.udpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.udpPipes = nil
	return firstErr
}

func (s *Supervisor) CloseClientPipes(clientID string) (int, error) {
	if clientID == "" {
		return 0, nil
	}
	closed := 0
	var firstErr error
	for i := len(s.tcpPipes) - 1; i >= 0; i-- {
		if s.tcpPipes[i].Pipe.Binding.ClientID != clientID {
			continue
		}
		if err := s.tcpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpPipes = append(s.tcpPipes[:i], s.tcpPipes[i+1:]...)
		closed++
	}
	for i := len(s.udpPipes) - 1; i >= 0; i-- {
		if s.udpPipes[i].Pipe.Binding.ClientID != clientID {
			continue
		}
		if err := s.udpPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.udpPipes = append(s.udpPipes[:i], s.udpPipes[i+1:]...)
		closed++
	}
	for i := len(s.xrayPipes) - 1; i >= 0; i-- {
		if s.xrayPipes[i].Pipe.Binding.ClientID != clientID {
			continue
		}
		if err := s.xrayPipes[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.xrayPipes = append(s.xrayPipes[:i], s.xrayPipes[i+1:]...)
		closed++
	}
	return closed, firstErr
}

func (s *Supervisor) UDPPipes() []*UDPPipeHandle {
	out := make([]*UDPPipeHandle, len(s.udpPipes))
	copy(out, s.udpPipes)
	return out
}

func (s *Supervisor) TCPPipes() []*TCPPipeHandle {
	out := make([]*TCPPipeHandle, len(s.tcpPipes))
	copy(out, s.tcpPipes)
	return out
}

func (s *Supervisor) XrayPipes() []*XrayPipeHandle {
	out := make([]*XrayPipeHandle, len(s.xrayPipes))
	copy(out, s.xrayPipes)
	return out
}

func (s *Supervisor) XrayStates() []xrayruntime.State {
	if s.xray == nil {
		return nil
	}
	state := s.xray.State()
	if state.EndpointCount == 0 && !state.Running {
		return nil
	}
	return []xrayruntime.State{state}
}

func findRuntimeDevice(devices []config.RuntimeDevice, id string) (config.RuntimeDevice, bool) {
	for _, device := range devices {
		if device.ID == id {
			return device, true
		}
	}
	return config.RuntimeDevice{}, false
}
