package panel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"tapx/internal/config"
	"tapx/internal/model"
)

const (
	KindDevices    = "devices"
	KindListeners  = "listeners"
	KindConnectors = "connectors"
	KindClients    = "clients"
	KindRoutes     = "routes"
	KindVKeys      = "vkeys"
	KindAddresses  = "addresses"
	KindXray       = "xrayProfiles"
	KindSettings   = "settings"
)

var (
	ErrUnknownKind = errors.New("unknown object kind")
	ErrNotFound    = errors.New("object not found")
	ErrIDMismatch  = errors.New("object id does not match path")
)

type Store struct {
	mu sync.Mutex
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("db path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS tapx_objects (
			kind TEXT NOT NULL,
			id TEXT NOT NULL,
			payload TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (kind, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tapx_objects_kind ON tapx_objects(kind, id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) LoadConfig(ctx context.Context) (config.RuntimeConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadConfig(ctx)
}

func (s *Store) loadConfig(ctx context.Context) (config.RuntimeConfig, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind, payload FROM tapx_objects ORDER BY kind, id`)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	defer rows.Close()

	var cfg config.RuntimeConfig
	for rows.Next() {
		var kind string
		var payload []byte
		if err := rows.Scan(&kind, &payload); err != nil {
			return config.RuntimeConfig{}, err
		}
		if err := appendObject(&cfg, kind, payload); err != nil {
			return config.RuntimeConfig{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return config.RuntimeConfig{}, err
	}
	return cfg, nil
}

func (s *Store) ReplaceConfig(ctx context.Context, cfg config.RuntimeConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceConfig(ctx, cfg)
}

func (s *Store) replaceConfig(ctx context.Context, cfg config.RuntimeConfig) error {
	if err := config.ValidateForSave(cfg); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM tapx_objects`); err != nil {
		return err
	}
	if err := insertConfig(ctx, tx, cfg); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListObjects(ctx context.Context, kind string) ([]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !knownKind(kind) {
		return nil, ErrUnknownKind
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM tapx_objects WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []json.RawMessage
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		items = append(items, json.RawMessage(append([]byte(nil), payload...)))
	}
	return items, rows.Err()
}

func (s *Store) GetObject(ctx context.Context, kind, id string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !knownKind(kind) {
		return nil, ErrUnknownKind
	}
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM tapx_objects WHERE kind = ? AND id = ?`, kind, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func (s *Store) UpsertObject(ctx context.Context, kind, id string, raw []byte) (config.RuntimeConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !knownKind(kind) {
		return config.RuntimeConfig{}, ErrUnknownKind
	}
	if id == "" {
		return config.RuntimeConfig{}, fmt.Errorf("object id is required")
	}
	objID, payload, err := normalizeObject(kind, id, raw)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	if objID != id {
		return config.RuntimeConfig{}, ErrIDMismatch
	}

	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	if err := replaceObject(&cfg, kind, payload); err != nil {
		return config.RuntimeConfig{}, err
	}
	if err := config.ValidateForSave(cfg); err != nil {
		return config.RuntimeConfig{}, err
	}
	if err := s.replaceConfig(ctx, cfg); err != nil {
		return config.RuntimeConfig{}, err
	}
	return cfg, nil
}

func (s *Store) DeleteObject(ctx context.Context, kind, id string) (config.RuntimeConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !knownKind(kind) {
		return config.RuntimeConfig{}, ErrUnknownKind
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	removed := removeObject(&cfg, kind, id)
	if !removed {
		return config.RuntimeConfig{}, ErrNotFound
	}
	if err := config.ValidateForSave(cfg); err != nil {
		return config.RuntimeConfig{}, err
	}
	if err := s.replaceConfig(ctx, cfg); err != nil {
		return config.RuntimeConfig{}, err
	}
	return cfg, nil
}

func insertConfig(ctx context.Context, tx *sql.Tx, cfg config.RuntimeConfig) error {
	for _, item := range cfg.Devices {
		if err := insertObject(ctx, tx, KindDevices, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Listeners {
		if err := insertObject(ctx, tx, KindListeners, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Connectors {
		if err := insertObject(ctx, tx, KindConnectors, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Clients {
		if err := insertObject(ctx, tx, KindClients, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Routes {
		if err := insertObject(ctx, tx, KindRoutes, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.VKeys {
		if err := insertObject(ctx, tx, KindVKeys, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Addresses {
		if err := insertObject(ctx, tx, KindAddresses, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.XrayProfiles {
		if err := insertObject(ctx, tx, KindXray, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Settings {
		if err := insertObject(ctx, tx, KindSettings, item.ID, item); err != nil {
			return err
		}
	}
	return nil
}

func insertObject(ctx context.Context, tx *sql.Tx, kind, id string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO tapx_objects(kind, id, payload, updated_at) VALUES (?, ?, ?, ?)`,
		kind, id, string(payload), time.Now().Unix(),
	)
	return err
}

func knownKind(kind string) bool {
	switch kind {
	case KindDevices, KindListeners, KindConnectors, KindClients, KindRoutes, KindVKeys, KindAddresses, KindXray, KindSettings:
		return true
	default:
		return false
	}
}

func normalizeObject(kind, fallbackID string, raw []byte) (string, json.RawMessage, error) {
	switch kind {
	case KindDevices:
		var item model.Device
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindListeners:
		var item model.Listener
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindConnectors:
		var item model.Connector
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindClients:
		var item model.Client
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindRoutes:
		var item model.Route
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindVKeys:
		var item model.VKey
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindAddresses:
		var item model.AddressLimit
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindXray:
		var item model.XrayProfile
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	case KindSettings:
		var item model.Settings
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", nil, err
		}
		if item.ID == "" {
			item.ID = fallbackID
		}
		return marshalObject(item.ID, item)
	default:
		return "", nil, ErrUnknownKind
	}
}

func marshalObject(id string, value any) (string, json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	return id, payload, nil
}

func appendObject(cfg *config.RuntimeConfig, kind string, payload []byte) error {
	switch kind {
	case KindDevices:
		var item model.Device
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Devices = append(cfg.Devices, item)
	case KindListeners:
		var item model.Listener
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Listeners = append(cfg.Listeners, item)
	case KindConnectors:
		var item model.Connector
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Connectors = append(cfg.Connectors, item)
	case KindClients:
		var item model.Client
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Clients = append(cfg.Clients, item)
	case KindRoutes:
		var item model.Route
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Routes = append(cfg.Routes, item)
	case KindVKeys:
		var item model.VKey
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.VKeys = append(cfg.VKeys, item)
	case KindAddresses:
		var item model.AddressLimit
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Addresses = append(cfg.Addresses, item)
	case KindXray:
		var item model.XrayProfile
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.XrayProfiles = append(cfg.XrayProfiles, item)
	case KindSettings:
		var item model.Settings
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		cfg.Settings = append(cfg.Settings, item)
	default:
		return ErrUnknownKind
	}
	return nil
}

func replaceObject(cfg *config.RuntimeConfig, kind string, payload json.RawMessage) error {
	switch kind {
	case KindDevices:
		var item model.Device
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Devices {
			if cfg.Devices[i].ID == item.ID {
				cfg.Devices[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Devices = append(cfg.Devices, item)
		}
	case KindListeners:
		var item model.Listener
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Listeners {
			if cfg.Listeners[i].ID == item.ID {
				cfg.Listeners[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Listeners = append(cfg.Listeners, item)
		}
	case KindConnectors:
		var item model.Connector
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Connectors {
			if cfg.Connectors[i].ID == item.ID {
				cfg.Connectors[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Connectors = append(cfg.Connectors, item)
		}
	case KindClients:
		var item model.Client
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Clients {
			if cfg.Clients[i].ID == item.ID {
				cfg.Clients[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Clients = append(cfg.Clients, item)
		}
	case KindRoutes:
		var item model.Route
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Routes {
			if cfg.Routes[i].ID == item.ID {
				cfg.Routes[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Routes = append(cfg.Routes, item)
		}
	case KindVKeys:
		var item model.VKey
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.VKeys {
			if cfg.VKeys[i].ID == item.ID {
				cfg.VKeys[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.VKeys = append(cfg.VKeys, item)
		}
	case KindAddresses:
		var item model.AddressLimit
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Addresses {
			if cfg.Addresses[i].ID == item.ID {
				cfg.Addresses[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Addresses = append(cfg.Addresses, item)
		}
	case KindXray:
		var item model.XrayProfile
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.XrayProfiles {
			if cfg.XrayProfiles[i].ID == item.ID {
				cfg.XrayProfiles[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.XrayProfiles = append(cfg.XrayProfiles, item)
		}
	case KindSettings:
		var item model.Settings
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		replaced := false
		for i := range cfg.Settings {
			if cfg.Settings[i].ID == item.ID {
				cfg.Settings[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Settings = append(cfg.Settings, item)
		}
	default:
		return ErrUnknownKind
	}
	return nil
}

func removeObject(cfg *config.RuntimeConfig, kind, id string) bool {
	switch kind {
	case KindDevices:
		for i := range cfg.Devices {
			if cfg.Devices[i].ID == id {
				cfg.Devices = append(cfg.Devices[:i], cfg.Devices[i+1:]...)
				return true
			}
		}
	case KindListeners:
		for i := range cfg.Listeners {
			if cfg.Listeners[i].ID == id {
				cfg.Listeners = append(cfg.Listeners[:i], cfg.Listeners[i+1:]...)
				return true
			}
		}
	case KindConnectors:
		for i := range cfg.Connectors {
			if cfg.Connectors[i].ID == id {
				cfg.Connectors = append(cfg.Connectors[:i], cfg.Connectors[i+1:]...)
				return true
			}
		}
	case KindClients:
		for i := range cfg.Clients {
			if cfg.Clients[i].ID == id {
				cfg.Clients = append(cfg.Clients[:i], cfg.Clients[i+1:]...)
				return true
			}
		}
	case KindRoutes:
		for i := range cfg.Routes {
			if cfg.Routes[i].ID == id {
				cfg.Routes = append(cfg.Routes[:i], cfg.Routes[i+1:]...)
				return true
			}
		}
	case KindVKeys:
		for i := range cfg.VKeys {
			if cfg.VKeys[i].ID == id {
				cfg.VKeys = append(cfg.VKeys[:i], cfg.VKeys[i+1:]...)
				return true
			}
		}
	case KindAddresses:
		for i := range cfg.Addresses {
			if cfg.Addresses[i].ID == id {
				cfg.Addresses = append(cfg.Addresses[:i], cfg.Addresses[i+1:]...)
				return true
			}
		}
	case KindXray:
		for i := range cfg.XrayProfiles {
			if cfg.XrayProfiles[i].ID == id {
				cfg.XrayProfiles = append(cfg.XrayProfiles[:i], cfg.XrayProfiles[i+1:]...)
				return true
			}
		}
	case KindSettings:
		for i := range cfg.Settings {
			if cfg.Settings[i].ID == id {
				cfg.Settings = append(cfg.Settings[:i], cfg.Settings[i+1:]...)
				return true
			}
		}
	}
	return false
}
