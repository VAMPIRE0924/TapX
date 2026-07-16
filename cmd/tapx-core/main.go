package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	debugCounters := fs.Bool("debug-counters", false, "print worker counters once per second")
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
	fmt.Printf("runtime generated: devices=%d udp=%d udpDispatch=%d tcp=%d xrayEndpoints=%d xrayPipes=%d routes=%d\n",
		len(runtime.Devices), len(runtime.UDPPipes), len(runtime.UDPDispatches), len(runtime.TCPPipes), xrayruntime.CountEndpoints(runtime), len(runtime.XrayPipes), len(runtime.Routes))
	if *checkOnly {
		return nil
	}

	supervisor := core.NewSupervisor()
	if err := supervisor.Start(runtime); err != nil {
		return err
	}
	fmt.Println("runtime started")
	stopDebug := startDebugCounters(supervisor, *debugCounters)
	waitForStopSignal()
	stopDebug()
	if err := supervisor.Stop(); err != nil {
		return err
	}
	fmt.Println("runtime stopped")
	return nil
}

func startDebugCounters(supervisor *core.Supervisor, enabled bool) func() {
	if !enabled {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, pipe := range supervisor.UDPPipes() {
					counters := pipe.Counters()
					fmt.Printf("udp counters: endpoint=%s device=%s pathMTU=%d singleDatagramMTU=%d mss4=%d mss6=%d rxPackets=%d txPackets=%d rxBytes=%d txBytes=%d guardDrops=%d ioDrops=%d err=%v\n",
						pipe.Pipe.EndpointID, pipe.DeviceName,
						pipe.Pipe.ConfirmedPathMTU, pipe.Pipe.EffectiveNetworkMTU, pipe.Pipe.TCPMSSIPv4, pipe.Pipe.TCPMSSIPv6,
						counters.RXPackets, counters.TXPackets,
						counters.RXBytes, counters.TXBytes, counters.DropsGuard, counters.DropsIO, pipe.Err())
				}
				for _, pipe := range supervisor.TCPPipes() {
					counters := pipe.Counters()
					fmt.Printf("tcp counters: endpoint=%s device=%s rxPackets=%d txPackets=%d rxBytes=%d txBytes=%d guardDrops=%d ioDrops=%d err=%v\n",
						pipe.Pipe.EndpointID, pipe.DeviceName, counters.RXPackets, counters.TXPackets,
						counters.RXBytes, counters.TXBytes, counters.DropsGuard, counters.DropsIO, pipe.Err())
				}
				for _, pipe := range supervisor.XrayPipes() {
					counters := pipe.Counters()
					fmt.Printf("xray counters: endpoint=%s device=%s rxPackets=%d txPackets=%d rxBytes=%d txBytes=%d guardDrops=%d ioDrops=%d err=%v\n",
						pipe.Pipe.EndpointID, pipe.DeviceName, counters.RXPackets, counters.TXPackets,
						counters.RXBytes, counters.TXBytes, counters.DropsGuard, counters.DropsIO, pipe.Err())
				}
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
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
