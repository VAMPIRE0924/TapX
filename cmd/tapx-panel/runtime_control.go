package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tapx/internal/panel"
)

type runtimeControlRequest struct {
	Action string `json:"action"`
}

type runtimeControlResponse struct {
	State panel.RuntimeState `json:"state"`
	Error string             `json:"error,omitempty"`
}

func startRuntimeControl(socketPath string, store *panel.Store, manager *panel.RuntimeManager) (func(), error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, fmt.Errorf("create runtime control directory: %w", err)
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("runtime control path exists and is not a socket: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale runtime control socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect runtime control socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on runtime control socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("protect runtime control socket: %w", err)
	}

	var (
		closeOnce sync.Once
		done      = make(chan struct{})
	)
	go func() {
		defer close(done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go serveRuntimeControl(conn, store, manager)
		}
	}()
	return func() {
		closeOnce.Do(func() {
			_ = listener.Close()
			<-done
			_ = os.Remove(socketPath)
		})
	}, nil
}

func serveRuntimeControl(conn net.Conn, store *panel.Store, manager *panel.RuntimeManager) {
	defer conn.Close()
	var request runtimeControlRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(runtimeControlResponse{State: manager.State(), Error: "decode request: " + err.Error()})
		return
	}
	response := runtimeControlResponse{}
	switch strings.TrimSpace(request.Action) {
	case "status":
		response.State = manager.State()
	case "start", "restart":
		if err := restoreStoredRuntime(context.Background(), store, manager); err != nil {
			response.State = manager.State()
			response.Error = err.Error()
		} else {
			response.State = manager.State()
		}
	case "stop":
		response.State, response.Error = stopRuntime(manager)
	default:
		response.State = manager.State()
		response.Error = fmt.Sprintf("unsupported runtime action %q", request.Action)
	}
	_ = json.NewEncoder(conn).Encode(response)
}

func stopRuntime(manager *panel.RuntimeManager) (panel.RuntimeState, string) {
	state, err := manager.Stop()
	if err != nil {
		return state, err.Error()
	}
	return state, ""
}

func requestRuntimeControl(socketPath, action string) (runtimeControlResponse, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return runtimeControlResponse{}, fmt.Errorf("runtime control socket is required")
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return runtimeControlResponse{}, fmt.Errorf("connect to runtime control socket: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(conn).Encode(runtimeControlRequest{Action: strings.TrimSpace(action)}); err != nil {
		return runtimeControlResponse{}, fmt.Errorf("send runtime control request: %w", err)
	}
	var response runtimeControlResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return runtimeControlResponse{}, fmt.Errorf("read runtime control response: %w", err)
	}
	if response.Error != "" {
		return response, fmt.Errorf("%s", response.Error)
	}
	return response, nil
}
