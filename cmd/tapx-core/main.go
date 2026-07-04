package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tapx/internal/buildinfo"
	"tapx/internal/config"
	"tapx/internal/core"
	"tapx/internal/xrayruntime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "tapx-core: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tapx-core", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to runtime object JSON")
	checkOnly := fs.Bool("check", false, "validate and generate runtime without starting workers")
	version := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version {
		fmt.Printf("tapx-core %s\n", buildinfo.Version)
		return nil
	}
	if *configPath == "" {
		fmt.Printf("tapx-core %s\n", buildinfo.Version)
		fs.Usage()
		return nil
	}

	cfg, err := readRuntimeConfig(*configPath)
	if err != nil {
		return err
	}
	runtime, err := config.GenerateRuntime(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("runtime generated: devices=%d udp=%d tcp=%d xrayEndpoints=%d xrayPipes=%d routes=%d\n",
		len(runtime.Devices), len(runtime.UDPPipes), len(runtime.TCPPipes), xrayruntime.CountEndpoints(runtime), len(runtime.XrayPipes), len(runtime.Routes))
	if *checkOnly {
		return nil
	}

	supervisor := core.NewSupervisor()
	if err := supervisor.Start(runtime); err != nil {
		return err
	}
	fmt.Println("runtime started")
	waitForStopSignal()
	if err := supervisor.Stop(); err != nil {
		return err
	}
	fmt.Println("runtime stopped")
	return nil
}

func readRuntimeConfig(path string) (config.RuntimeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.RuntimeConfig{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg config.RuntimeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config.RuntimeConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func waitForStopSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	signal.Stop(ch)
}
