package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	listenInterface := fs.String("listen-interface", "", "optional Linux interface used to accept panel connections")
	basePath := fs.String("base-path", "", "optional secret HTTP base path, for example /tapx-abcd")
	dbDriver := fs.String("db-driver", envDefault("TAPX_DB_DRIVER", "sqlite"), "database driver: sqlite or postgres")
	dbSource := fs.String("db", envDefault("TAPX_DB_SOURCE", envDefault("TAPX_DB_PATH", "tapx.db")), "SQLite path or PostgreSQL DSN")
	checkOnly := fs.Bool("check", false, "open database and validate stored config without starting HTTP")
	checkListen := fs.Bool("check-listen", false, "check whether the panel listen socket can be opened and exit")
	exportBackup := fs.String("export-backup", "", "write a consistent portable TapX database backup and exit")
	restoreBackup := fs.String("restore-backup", "", "restore a portable TapX database backup and exit")
	exportRuntimeConfig := fs.String("export-runtime-config", "", "write the validated runtime object JSON stored in the database and exit")
	hashPasswordStdin := fs.Bool("hash-password-stdin", false, "read password from stdin and print a panel password hash")
	initAdmin := fs.Bool("init-admin", false, "initialize panel admin auth settings in the database and exit")
	setPanelEndpoint := fs.Bool("set-panel-endpoint", false, "update the panel listen address and base path in the database and exit")
	adminUsername := fs.String("admin-username", "admin", "admin username for -init-admin")
	adminPassword := fs.String("admin-password", "", "admin password for -init-admin")
	adminPasswordHash := fs.String("admin-password-hash", "", "pre-hashed admin password for -init-admin")
	panelCertFile := fs.String("panel-cert-file", "", "panel TLS certificate file for -init-admin")
	panelKeyFile := fs.String("panel-key-file", "", "panel TLS private key file for -init-admin")
	disablePanelHTTPS := fs.Bool("disable-panel-https", false, "disable panel HTTPS for -set-panel-endpoint")
	version := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	listenFlagSet := flagWasSet(fs, "listen")
	basePathFlagSet := flagWasSet(fs, "base-path")
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
	if *checkListen {
		listener, err := listenPanel(*listen, *listenInterface)
		if err != nil {
			return err
		}
		defer listener.Close()
		fmt.Printf("tapx-panel listen available: %s interface=%s\n", *listen, displayListenInterface(*listenInterface))
		return nil
	}

	store, err := panel.OpenStoreWithOptions(panel.StoreOptions{
		Driver:     panel.DatabaseDriver(*dbDriver),
		DataSource: *dbSource,
	})
	if err != nil {
		return err
	}
	defer store.Close()
	if strings.TrimSpace(*exportBackup) != "" {
		if err := exportDatabaseBackup(context.Background(), store, *exportBackup); err != nil {
			return err
		}
		fmt.Printf("tapx-panel backup exported: %s\n", *exportBackup)
		return nil
	}
	if strings.TrimSpace(*restoreBackup) != "" {
		if err := store.RestoreDatabaseFile(context.Background(), *restoreBackup); err != nil {
			return err
		}
		fmt.Printf("tapx-panel backup restored: %s\n", *restoreBackup)
		return nil
	}
	if strings.TrimSpace(*exportRuntimeConfig) != "" {
		if err := writeRuntimeConfig(context.Background(), store, *exportRuntimeConfig); err != nil {
			return err
		}
		fmt.Printf("tapx-panel runtime config exported: %s\n", *exportRuntimeConfig)
		return nil
	}

	if *initAdmin {
		if err := initAdminSettings(context.Background(), store, *listen, *basePath, *adminUsername, *adminPassword, *adminPasswordHash, *panelCertFile, *panelKeyFile); err != nil {
			return err
		}
		fmt.Printf("tapx-panel admin initialized: username=%s listen=%s basePath=%s\n", *adminUsername, *listen, displayBasePath(*basePath))
		return nil
	}
	if *setPanelEndpoint {
		if err := updatePanelEndpoint(context.Background(), store, *listen, *basePath, *panelCertFile, *panelKeyFile, *disablePanelHTTPS); err != nil {
			return err
		}
		fmt.Printf("tapx-panel endpoint updated: listen=%s basePath=%s\n", *listen, displayBasePath(*basePath))
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
	restartCh := make(chan struct{}, 1)

	panelServer, err := loadPanelServerSettings(context.Background(), store, *listen, listenFlagSet)
	if err != nil {
		return err
	}
	effectiveBasePath := *basePath
	if !basePathFlagSet && panelServer.BasePath != "" {
		effectiveBasePath = panelServer.BasePath
	}
	handler := panel.NewServerWithOptions(store, panel.ServerOptions{
		BasePath: effectiveBasePath,
		Restart: func() error {
			select {
			case restartCh <- struct{}{}:
			default:
			}
			return nil
		},
	}, runtimeManager).Handler()
	if panelServer.Domain != "" {
		handler = requirePanelHost(handler, panelServer.Domain)
	}
	server := &http.Server{
		Addr:              panelServer.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	listener, err := listenPanel(panelServer.Listen, *listenInterface)
	if err != nil {
		return err
	}
	go func() {
		fmt.Printf("tapx-panel %s listening on %s://%s interface=%s\n", buildinfo.Version, panelServer.Scheme(), panelServer.Listen, displayListenInterface(*listenInterface))
		var err error
		if panelServer.HTTPS {
			err = server.ServeTLS(listener, panelServer.CertFile, panelServer.KeyFile)
		} else {
			err = server.Serve(listener)
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
	case <-restartCh:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return err
		}
		return fmt.Errorf("panel restart requested")
	}
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

type panelServerSettings struct {
	Listen   string
	Domain   string
	BasePath string
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
		out.Domain = strings.TrimSpace(item.PanelDomain)
		out.BasePath = strings.TrimSpace(item.PanelBasePath)
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

func requirePanelHost(next http.Handler, domain string) http.Handler {
	expected := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.TrimSpace(r.Host)
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
		host = strings.Trim(strings.TrimSuffix(strings.ToLower(host), "."), "[]")
		if host != expected {
			http.Error(w, "panel host is not allowed", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func initAdminSettings(ctx context.Context, store *panel.Store, listen, basePath, username, password, passwordHash, certFile, keyFile string) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("admin username is required")
	}
	hash := strings.TrimSpace(passwordHash)
	if hash != "" {
		if err := panel.ValidatePanelPasswordHash(hash); err != nil {
			return fmt.Errorf("admin password hash: %w", err)
		}
	} else {
		if password == "" {
			return fmt.Errorf("admin password or password hash is required")
		}
		var err error
		hash, err = panel.HashPanelPassword(password)
		if err != nil {
			return err
		}
	}
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	settings := model.Settings{
		ID:                  "global",
		Enabled:             true,
		Name:                "Default",
		SessionTTLSecond:    86400,
		LogLevel:            "info",
		StatsIntervalSecond: 5,
		BackupDir:           "/var/lib/tapx/backups",
		DataDir:             "/var/lib/tapx",
		OpenWrtBuildTarget:  "x86-64",
	}
	settingsIndex := -1
	if len(cfg.Settings) > 0 {
		settingsIndex = 0
		settings = cfg.Settings[0]
	}
	settings.Enabled = true
	settings.PanelListen = strings.TrimSpace(listen)
	settings.PanelBasePath = displayBasePath(basePath)
	settings.PanelAuthEnabled = true
	settings.AdminUsername = strings.TrimSpace(username)
	settings.AdminPasswordHash = hash
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if (certFile == "") != (keyFile == "") {
		return fmt.Errorf("panel certificate and private key must be configured together")
	}
	if certFile != "" {
		if _, err := os.Stat(certFile); err != nil {
			return fmt.Errorf("panel certificate: %w", err)
		}
		if _, err := os.Stat(keyFile); err != nil {
			return fmt.Errorf("panel private key: %w", err)
		}
		settings.PanelHTTPS = true
		settings.PanelCertFile = certFile
		settings.PanelKeyFile = keyFile
	}
	if settings.ID == "" {
		settings.ID = "global"
	}
	if settings.Name == "" {
		settings.Name = "Default"
	}
	if settings.SessionTTLSecond <= 0 {
		settings.SessionTTLSecond = 86400
	}
	if settings.LogLevel == "" {
		settings.LogLevel = "info"
	}
	if settings.StatsIntervalSecond <= 0 {
		settings.StatsIntervalSecond = 5
	}
	if settings.BackupDir == "" {
		settings.BackupDir = "/var/lib/tapx/backups"
	}
	if settings.DataDir == "" {
		settings.DataDir = "/var/lib/tapx"
	}
	if settings.OpenWrtBuildTarget == "" {
		settings.OpenWrtBuildTarget = "x86-64"
	}
	if settingsIndex < 0 {
		cfg.Settings = append(cfg.Settings, settings)
	} else {
		cfg.Settings[settingsIndex] = settings
	}
	return store.ReplaceConfig(ctx, cfg)
}

func updatePanelEndpoint(ctx context.Context, store *panel.Store, listen, basePath, certFile, keyFile string, disableHTTPS bool) error {
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	if len(cfg.Settings) == 0 || !cfg.Settings[0].PanelAuthEnabled || strings.TrimSpace(cfg.Settings[0].AdminPasswordHash) == "" {
		return fmt.Errorf("panel credentials are not initialized")
	}
	cfg.Settings[0].Enabled = true
	cfg.Settings[0].PanelListen = strings.TrimSpace(listen)
	cfg.Settings[0].PanelBasePath = displayBasePath(basePath)
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if disableHTTPS && (certFile != "" || keyFile != "") {
		return fmt.Errorf("panel HTTPS cannot be disabled and configured at the same time")
	}
	if (certFile == "") != (keyFile == "") {
		return fmt.Errorf("panel certificate and private key must be configured together")
	}
	if disableHTTPS {
		cfg.Settings[0].PanelHTTPS = false
		cfg.Settings[0].PanelCertFile = ""
		cfg.Settings[0].PanelKeyFile = ""
	} else if certFile != "" {
		if _, err := os.Stat(certFile); err != nil {
			return fmt.Errorf("panel certificate: %w", err)
		}
		if _, err := os.Stat(keyFile); err != nil {
			return fmt.Errorf("panel private key: %w", err)
		}
		cfg.Settings[0].PanelHTTPS = true
		cfg.Settings[0].PanelCertFile = certFile
		cfg.Settings[0].PanelKeyFile = keyFile
	}
	return store.ReplaceConfig(ctx, cfg)
}

func exportDatabaseBackup(ctx context.Context, store *panel.Store, destination string) error {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return fmt.Errorf("backup destination is required")
	}
	source, _, cleanup, err := store.OpenDatabaseBackup(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".tapx-backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, source); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return os.Chmod(destination, 0o600)
}

func writeRuntimeConfig(ctx context.Context, store *panel.Store, destination string) error {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return fmt.Errorf("runtime config destination is required")
	}
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	if err := config.ValidateForApply(cfg); err != nil {
		return fmt.Errorf("runtime config is not applicable: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".tapx-runtime-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return os.Chmod(destination, 0o600)
}

func displayListenInterface(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return "all"
}

func displayBasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func stopSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}
