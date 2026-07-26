//go:build !linux

package core

import "tapx/internal/config"

type deviceFabric struct{}

func newDeviceFabric(*config.GeneratedRuntime) (*deviceFabric, error) { return &deviceFabric{}, nil }
func (f *deviceFabric) Close() error                                  { return nil }
func (f *deviceFabric) Port(string) *sharedRuntimeDevice              { return nil }
