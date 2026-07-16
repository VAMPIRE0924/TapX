package panel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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
	mu      sync.Mutex
	db      *sql.DB
	dialect DatabaseDriver
}

type DatabaseDriver string

const (
	DatabaseSQLite   DatabaseDriver = "sqlite"
	DatabasePostgres DatabaseDriver = "postgres"
)

type StoreOptions struct {
	Driver     DatabaseDriver
	DataSource string
}

func OpenStore(path string) (*Store, error) {
	return OpenStoreWithOptions(StoreOptions{Driver: DatabaseSQLite, DataSource: path})
}

func OpenStoreWithOptions(options StoreOptions) (*Store, error) {
	driver := DatabaseDriver(strings.ToLower(strings.TrimSpace(string(options.Driver))))
	if driver == "" {
		driver = DatabaseSQLite
	}
	dataSource := strings.TrimSpace(options.DataSource)
	if dataSource == "" {
		return nil, fmt.Errorf("database data source is required")
	}

	sqlDriver := "sqlite3"
	switch driver {
	case DatabaseSQLite:
		if dataSource != ":memory:" {
			if err := os.MkdirAll(filepath.Dir(dataSource), 0o700); err != nil {
				return nil, fmt.Errorf("create db directory: %w", err)
			}
		}
	case DatabasePostgres:
		sqlDriver = "pgx"
	default:
		return nil, fmt.Errorf("unsupported database driver %q", options.Driver)
	}

	db, err := sql.Open(sqlDriver, dataSource)
	if err != nil {
		return nil, err
	}
	if driver == DatabaseSQLite {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect %s database: %w", driver, err)
	}
	store := &Store{db: db, dialect: driver}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if driver == DatabaseSQLite && dataSource != ":memory:" {
		if err := secureSQLiteFiles(dataSource); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) DatabaseDriver() DatabaseDriver {
	return s.dialect
}

func secureSQLiteFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0o600); err != nil {
			if errors.Is(err, os.ErrNotExist) && candidate != path {
				continue
			}
			return fmt.Errorf("secure SQLite file %s: %w", candidate, err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := make([]string, 0, 8)
	if s.dialect == DatabaseSQLite {
		stmts = append(stmts, `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`)
	}
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS tapx_objects (
			kind TEXT NOT NULL,
			id TEXT NOT NULL,
			payload TEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (kind, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tapx_objects_kind ON tapx_objects(kind, id)`,
		`CREATE TABLE IF NOT EXISTS tapx_integrations (
			name TEXT NOT NULL PRIMARY KEY,
			payload TEXT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tapx_logs (
			seq BIGINT NOT NULL PRIMARY KEY,
			time TEXT NOT NULL,
			level TEXT NOT NULL,
			action TEXT NOT NULL,
			message TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tapx_metrics (
			sampled_at BIGINT NOT NULL PRIMARY KEY,
			cpu DOUBLE PRECISION NOT NULL,
			memory DOUBLE PRECISION NOT NULL,
			embedded_xray BIGINT NOT NULL,
			external_xray BIGINT NOT NULL,
			tapx BIGINT NOT NULL,
			rx BIGINT NOT NULL,
			tx BIGINT NOT NULL,
			drops BIGINT NOT NULL
		)`,
	)
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if err := s.ensureMetricDetailsColumn(ctx); err != nil {
		return fmt.Errorf("migrate metric details: %w", err)
	}
	return nil
}

func (s *Store) ensureMetricDetailsColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT details FROM tapx_metrics LIMIT 0`)
	if err == nil {
		return rows.Close()
	}
	statement := `ALTER TABLE tapx_metrics ADD COLUMN details TEXT NOT NULL DEFAULT '{}'`
	if s.dialect == DatabasePostgres {
		statement = `ALTER TABLE tapx_metrics ADD COLUMN IF NOT EXISTS details TEXT NOT NULL DEFAULT '{}'`
	}
	_, alterErr := s.db.ExecContext(ctx, statement)
	return alterErr
}

func (s *Store) bind(query string) string {
	return bindQuery(s.dialect, query)
}

func bindQuery(dialect DatabaseDriver, query string) string {
	if dialect != DatabasePostgres {
		return query
	}
	var out strings.Builder
	out.Grow(len(query) + 16)
	parameter := 1
	for _, char := range query {
		if char == '?' {
			fmt.Fprintf(&out, "$%d", parameter)
			parameter++
			continue
		}
		out.WriteRune(char)
	}
	return out.String()
}

func (s *Store) GetIntegration(ctx context.Context, name string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		return nil, fmt.Errorf("integration name is required")
	}
	var payload []byte
	err := s.db.QueryRowContext(ctx, s.bind(`SELECT payload FROM tapx_integrations WHERE name = ?`), name).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(append([]byte(nil), payload...)), nil
}

func (s *Store) SetIntegration(ctx context.Context, name string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		return fmt.Errorf("integration name is required")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.bind(`
		INSERT INTO tapx_integrations(name, payload, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`),
		name, string(payload), time.Now().Unix(),
	)
	return err
}

func (s *Store) DeleteIntegration(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(ctx, s.bind(`DELETE FROM tapx_integrations WHERE name = ?`), name)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListIntegrations(ctx context.Context) (map[string]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.QueryContext(ctx, `SELECT name, payload FROM tapx_integrations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[string]json.RawMessage)
	for rows.Next() {
		var name string
		var payload []byte
		if err := rows.Scan(&name, &payload); err != nil {
			return nil, err
		}
		items[name] = json.RawMessage(append([]byte(nil), payload...))
	}
	return items, rows.Err()
}

func (s *Store) ReplaceConfigAndIntegrations(ctx context.Context, cfg config.RuntimeConfig, integrations map[string]json.RawMessage, logs []LogEvent, metrics []DashboardMetricSample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if err := insertConfig(ctx, tx, s.dialect, cfg); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tapx_integrations`); err != nil {
		return err
	}
	for name, payload := range integrations {
		if name == "" || !json.Valid(payload) {
			return fmt.Errorf("invalid integration backup entry %q", name)
		}
		if _, err := tx.ExecContext(ctx,
			s.bind(`INSERT INTO tapx_integrations(name, payload, updated_at) VALUES (?, ?, ?)`),
			name, string(payload), time.Now().Unix(),
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tapx_logs`); err != nil {
		return err
	}
	if len(logs) > defaultLogLimit {
		logs = logs[len(logs)-defaultLogLimit:]
	}
	for _, event := range logs {
		if _, err := tx.ExecContext(ctx,
			s.bind(`INSERT INTO tapx_logs(seq, time, level, action, message) VALUES (?, ?, ?, ?, ?)`),
			event.Seq, event.Time, event.Level, event.Action, event.Message,
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tapx_metrics`); err != nil {
		return err
	}
	if len(metrics) > defaultMetricLimit {
		metrics = metrics[len(metrics)-defaultMetricLimit:]
	}
	for _, sample := range metrics {
		if err := insertMetric(ctx, tx, s.dialect, sample); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AppendMetric(ctx context.Context, sample DashboardMetricSample, minimumInterval time.Duration, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = defaultMetricLimit
	}
	var latest int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sampled_at), 0) FROM tapx_metrics`).Scan(&latest)
	if err != nil {
		return err
	}
	if latest > 0 && sample.At-latest < minimumInterval.Milliseconds() {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertMetric(ctx, tx, s.dialect, sample); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		s.bind(`DELETE FROM tapx_metrics WHERE sampled_at NOT IN (SELECT sampled_at FROM tapx_metrics ORDER BY sampled_at DESC LIMIT ?)`),
		limit,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LoadMetrics(ctx context.Context, limit int) ([]DashboardMetricSample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = defaultMetricLimit
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT sampled_at, cpu, memory, embedded_xray, external_xray, tapx, rx, tx, drops, details
		FROM (SELECT sampled_at, cpu, memory, embedded_xray, external_xray, tapx, rx, tx, drops, details
			FROM tapx_metrics ORDER BY sampled_at DESC LIMIT ?)
			AS recent_metrics
		ORDER BY sampled_at`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DashboardMetricSample, 0)
	for rows.Next() {
		var item DashboardMetricSample
		var details string
		if err := rows.Scan(&item.At, &item.CPU, &item.Memory, &item.EmbeddedXray, &item.ExternalXray, &item.TapX, &item.RX, &item.TX, &item.Drops, &details); err != nil {
			return nil, err
		}
		if details != "" && details != "{}" {
			if err := json.Unmarshal([]byte(details), &item); err != nil {
				return nil, fmt.Errorf("decode metric details at %d: %w", item.At, err)
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func insertMetric(ctx context.Context, tx *sql.Tx, dialect DatabaseDriver, sample DashboardMetricSample) error {
	details, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	query := bindQuery(dialect, `
		INSERT INTO tapx_metrics(sampled_at, cpu, memory, embedded_xray, external_xray, tapx, rx, tx, drops, details)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(sampled_at) DO UPDATE SET
			cpu=excluded.cpu, memory=excluded.memory, embedded_xray=excluded.embedded_xray,
			external_xray=excluded.external_xray, tapx=excluded.tapx, rx=excluded.rx,
			tx=excluded.tx, drops=excluded.drops, details=excluded.details`)
	_, err = tx.ExecContext(ctx, query,
		sample.At, sample.CPU, sample.Memory, sample.EmbeddedXray, sample.ExternalXray, sample.TapX, sample.RX, sample.TX, sample.Drops, string(details),
	)
	return err
}

func (s *Store) AppendLog(ctx context.Context, event LogEvent, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = defaultLogLimit
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		s.bind(`INSERT INTO tapx_logs(seq, time, level, action, message) VALUES (?, ?, ?, ?, ?)`),
		event.Seq, event.Time, event.Level, event.Action, event.Message,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		s.bind(`DELETE FROM tapx_logs WHERE seq NOT IN (SELECT seq FROM tapx_logs ORDER BY seq DESC LIMIT ?)`),
		limit,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LoadLogs(ctx context.Context, limit int) ([]LogEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = defaultLogLimit
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT seq, time, level, action, message
		FROM (SELECT seq, time, level, action, message FROM tapx_logs ORDER BY seq DESC LIMIT ?)
			AS recent_logs
		ORDER BY seq`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]LogEvent, 0)
	for rows.Next() {
		var event LogEvent
		if err := rows.Scan(&event.Seq, &event.Time, &event.Level, &event.Action, &event.Message); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ClearLogs(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM tapx_logs`)
	return err
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
	if err := insertConfig(ctx, tx, s.dialect, cfg); err != nil {
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
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT payload FROM tapx_objects WHERE kind = ? ORDER BY id`), kind)
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
	err := s.db.QueryRowContext(ctx, s.bind(`SELECT payload FROM tapx_objects WHERE kind = ? AND id = ?`), kind, id).Scan(&payload)
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

func insertConfig(ctx context.Context, tx *sql.Tx, dialect DatabaseDriver, cfg config.RuntimeConfig) error {
	for _, item := range cfg.Devices {
		if err := insertObject(ctx, tx, dialect, KindDevices, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Listeners {
		if err := insertObject(ctx, tx, dialect, KindListeners, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Connectors {
		if err := insertObject(ctx, tx, dialect, KindConnectors, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Clients {
		if err := insertObject(ctx, tx, dialect, KindClients, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Routes {
		if err := insertObject(ctx, tx, dialect, KindRoutes, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.VKeys {
		if err := insertObject(ctx, tx, dialect, KindVKeys, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Addresses {
		if err := insertObject(ctx, tx, dialect, KindAddresses, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.XrayProfiles {
		if err := insertObject(ctx, tx, dialect, KindXray, item.ID, item); err != nil {
			return err
		}
	}
	for _, item := range cfg.Settings {
		if err := insertObject(ctx, tx, dialect, KindSettings, item.ID, item); err != nil {
			return err
		}
	}
	return nil
}

func insertObject(ctx context.Context, tx *sql.Tx, dialect DatabaseDriver, kind, id string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		bindQuery(dialect, `INSERT INTO tapx_objects(kind, id, payload, updated_at) VALUES (?, ?, ?, ?)`),
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
