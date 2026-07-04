package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tapx/internal/buildinfo"
	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/panel"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "tapx-panel: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tapx-panel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listen := fs.String("listen", "127.0.0.1:8080", "HTTP API listen address")
	basePath := fs.String("base-path", "", "optional secret HTTP base path, for example /tapx-abcd")
	dbPath := fs.String("db", "tapx.db", "SQLite database path")
	checkOnly := fs.Bool("check", false, "open database and validate stored config without starting HTTP")
	hashPasswordStdin := fs.Bool("hash-password-stdin", false, "read password from stdin and print a panel password hash")
	initAdmin := fs.Bool("init-admin", false, "initialize panel admin auth settings in the database and exit")
	adminUsername := fs.String("admin-username", "admin", "admin username for -init-admin")
	adminPassword := fs.String("admin-password", "", "admin password for -init-admin")
	version := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	listenFlagSet := flagWasSet(fs, "listen")
	if *version {
		fmt.Printf("tapx-panel %s\n", buildinfo.Version)
		return nil
	}
	if *hashPasswordStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		hash, err := panel.HashPanelPassword(strings.TrimRight(string(raw), "\r\n"))
		if err != nil {
			return err
		}
		fmt.Println(hash)
		return nil
	}

	store, err := panel.OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	if *initAdmin {
		if err := initAdminSettings(context.Background(), store, *listen, *adminUsername, *adminPassword); err != nil {
			return err
		}
		fmt.Printf("tapx-panel admin initialized: username=%s listen=%s basePath=%s\n", *adminUsername, *listen, displayBasePath(*basePath))
		return nil
	}

	if *checkOnly {
		cfg, err := store.LoadConfig(context.Background())
		if err != nil {
			return err
		}
		if err := config.ValidateForSave(cfg); err != nil {
			return err
		}
		fmt.Printf("tapx-panel db ok: devices=%d listeners=%d connectors=%d routes=%d\n",
			len(cfg.Devices), len(cfg.Listeners), len(cfg.Connectors), len(cfg.Routes))
		return nil
	}

	runtimeManager := panel.NewRuntimeManager()
	defer runtimeManager.Stop()

	panelServer, err := loadPanelServerSettings(context.Background(), store, *listen, listenFlagSet)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              panelServer.Listen,
		Handler:           panel.NewServerWithOptions(store, panel.ServerOptions{BasePath: *basePath}, runtimeManager).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("tapx-panel %s listening on %s://%s\n", buildinfo.Version, panelServer.Scheme(), panelServer.Listen)
		var err error
		if panelServer.HTTPS {
			err = server.ListenAndServeTLS(panelServer.CertFile, panelServer.KeyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-stopSignal():
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

type panelServerSettings struct {
	Listen   string
	HTTPS    bool
	CertFile string
	KeyFile  string
}

func (s panelServerSettings) Scheme() string {
	if s.HTTPS {
		return "https"
	}
	return "http"
}

func loadPanelServerSettings(ctx context.Context, store *panel.Store, fallbackListen string, listenOverride bool) (panelServerSettings, error) {
	out := panelServerSettings{Listen: strings.TrimSpace(fallbackListen)}
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return panelServerSettings{}, err
	}
	for _, item := range cfg.Settings {
		if !item.Enabled {
			continue
		}
		if !listenOverride && strings.TrimSpace(item.PanelListen) != "" {
			out.Listen = strings.TrimSpace(item.PanelListen)
		}
		out.HTTPS = item.PanelHTTPS
		out.CertFile = strings.TrimSpace(item.PanelCertFile)
		out.KeyFile = strings.TrimSpace(item.PanelKeyFile)
		break
	}
	if out.Listen == "" {
		return panelServerSettings{}, fmt.Errorf("panel listen address is required")
	}
	if out.HTTPS && (out.CertFile == "" || out.KeyFile == "") {
		return panelServerSettings{}, fmt.Errorf("panel HTTPS requires PanelCertFile and PanelKeyFile")
	}
	return out, nil
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	wasSet := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			wasSet = true
		}
	})
	return wasSet
}

func initAdminSettings(ctx context.Context, store *panel.Store, listen, username, password string) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("admin username is required")
	}
	if password == "" {
		return fmt.Errorf("admin password is required")
	}
	hash, err := panel.HashPanelPassword(password)
	if err != nil {
		return err
	}
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	settings := model.Settings{
		ID:                  "global",
		Enabled:             true,
		Name:                "Default",
		PanelListen:         listen,
		PanelAuthEnabled:    true,
		AdminUsername:       username,
		AdminPasswordHash:   hash,
		SessionTTLSecond:    86400,
		LogLevel:            "info",
		StatsIntervalSecond: 5,
		BackupDir:           "/var/lib/tapx/backups",
		DataDir:             "/var/lib/tapx",
		OpenWrtBuildTarget:  "x86-64",
	}
	replaced := false
	for i := range cfg.Settings {
		if cfg.Settings[i].ID == settings.ID {
			settings.PanelHTTPS = cfg.Settings[i].PanelHTTPS
			settings.PanelCertFile = cfg.Settings[i].PanelCertFile
			settings.PanelKeyFile = cfg.Settings[i].PanelKeyFile
			settings.ExternalXrayPath = cfg.Settings[i].ExternalXrayPath
			settings.AdvancedJSON = cfg.Settings[i].AdvancedJSON
			settings.Remark = cfg.Settings[i].Remark
			cfg.Settings[i] = settings
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Settings = append(cfg.Settings, settings)
	}
	return store.ReplaceConfig(ctx, cfg)
}

func displayBasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/"
	}
	return path
}

func stopSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}
