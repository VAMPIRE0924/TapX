package panel

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"

	"tapx/internal/config"
)

const sqliteHeader = "SQLite format 3\x00"

// BackupDatabase returns a consistent, self-contained TapX SQLite database.
// SQLite stores use the online backup API. PostgreSQL stores are exported into
// the same portable application database so backup and restore keep one format.
func (s *Store) BackupDatabase(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, cleanup, err := s.createDatabaseBackupLocked(ctx)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(raw, []byte(sqliteHeader)) {
		return nil, fmt.Errorf("generated backup is not a SQLite database")
	}
	return raw, nil
}

// OpenDatabaseBackup returns a seekable snapshot for streaming to a client.
// The caller must invoke cleanup after the response has been written.
func (s *Store) OpenDatabaseBackup(ctx context.Context) (*os.File, int64, func(), error) {
	s.mu.Lock()
	path, cleanup, err := s.createDatabaseBackupLocked(ctx)
	s.mu.Unlock()
	if err != nil {
		return nil, 0, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, nil, err
	}
	return file, info.Size(), func() {
		_ = file.Close()
		cleanup()
	}, nil
}

// ValidateDatabaseBackup verifies an uploaded database without changing the
// live store. RestoreDatabase repeats this check to avoid a validation/use gap.
func (s *Store) ValidateDatabaseBackup(ctx context.Context, raw []byte) error {
	_, cleanup, err := openBackupDatabase(ctx, raw)
	if cleanup != nil {
		defer cleanup()
	}
	return err
}

// RestoreDatabase restores a portable TapX .db into the active backend.
func (s *Store) RestoreDatabase(ctx context.Context, raw []byte) error {
	source, cleanup, err := openBackupDatabase(ctx, raw)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	return s.restoreDatabaseFrom(ctx, source)
}

// RestoreDatabaseFile restores a validated SQLite upload without retaining the
// complete upload in memory.
func (s *Store) RestoreDatabaseFile(ctx context.Context, path string) error {
	source, cleanup, err := openBackupDatabasePath(ctx, path)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	return s.restoreDatabaseFrom(ctx, source)
}

func (s *Store) restoreDatabaseFrom(ctx context.Context, source *sql.DB) error {
	if s.dialect == DatabasePostgres {
		snapshot, err := readPortableSnapshot(ctx, &Store{db: source, dialect: DatabaseSQLite})
		if err != nil {
			return err
		}
		return s.ReplaceConfigAndIntegrations(ctx, snapshot.config, snapshot.integrations, snapshot.logs, snapshot.metrics)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rollbackPath, rollbackPathCleanup, err := createBackupDatabaseLocked(ctx, s.db)
	if err != nil {
		return fmt.Errorf("snapshot current database: %w", err)
	}
	defer rollbackPathCleanup()
	rollback, rollbackCleanup, err := openBackupDatabasePath(ctx, rollbackPath)
	if rollbackCleanup != nil {
		defer rollbackCleanup()
	}
	if err != nil {
		return fmt.Errorf("open rollback database: %w", err)
	}

	if err := copySQLiteDatabase(ctx, s.db, source); err != nil {
		_ = copySQLiteDatabase(context.Background(), s.db, rollback)
		return fmt.Errorf("restore database: %w", err)
	}
	if err := s.Migrate(ctx); err != nil {
		_ = copySQLiteDatabase(context.Background(), s.db, rollback)
		return fmt.Errorf("migrate restored database: %w", err)
	}
	if err := validateStoreConfig(ctx, s.db); err != nil {
		_ = copySQLiteDatabase(context.Background(), s.db, rollback)
		return fmt.Errorf("validate restored database: %w", err)
	}
	return nil
}

type portableSnapshot struct {
	config       config.RuntimeConfig
	integrations map[string]json.RawMessage
	logs         []LogEvent
	metrics      []DashboardMetricSample
}

func readPortableSnapshot(ctx context.Context, store *Store) (portableSnapshot, error) {
	cfg, err := store.LoadConfig(ctx)
	if err != nil {
		return portableSnapshot{}, err
	}
	integrations, err := store.ListIntegrations(ctx)
	if err != nil {
		return portableSnapshot{}, err
	}
	logs, err := store.LoadLogs(ctx, defaultLogLimit)
	if err != nil {
		return portableSnapshot{}, err
	}
	metrics, err := store.LoadMetrics(ctx, defaultMetricLimit)
	if err != nil {
		return portableSnapshot{}, err
	}
	return portableSnapshot{config: cfg, integrations: integrations, logs: logs, metrics: metrics}, nil
}

func (s *Store) createDatabaseBackupLocked(ctx context.Context) (string, func(), error) {
	if s.dialect == DatabaseSQLite {
		return createBackupDatabaseLocked(ctx, s.db)
	}
	snapshotStore := &Store{db: s.db, dialect: s.dialect}
	snapshot, err := readPortableSnapshot(ctx, snapshotStore)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "tapx-db-backup-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "tapx.db")
	destination, err := OpenStore(path)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if err := destination.ReplaceConfigAndIntegrations(ctx, snapshot.config, snapshot.integrations, snapshot.logs, snapshot.metrics); err != nil {
		_ = destination.Close()
		cleanup()
		return "", nil, err
	}
	if err := destination.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func createBackupDatabaseLocked(ctx context.Context, source *sql.DB) (string, func(), error) {
	dir, err := os.MkdirTemp("", "tapx-db-backup-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	path := filepath.Join(dir, "tapx.db")
	destination, err := sql.Open("sqlite3", path)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	destination.SetMaxOpenConns(1)
	if err := copySQLiteDatabase(ctx, destination, source); err != nil {
		_ = destination.Close()
		cleanup()
		return "", nil, err
	}
	if err := destination.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	header := make([]byte, len(sqliteHeader))
	_, readErr := io.ReadFull(file, header)
	_ = file.Close()
	if readErr != nil || !bytes.Equal(header, []byte(sqliteHeader)) {
		cleanup()
		return "", nil, fmt.Errorf("generated backup is not a SQLite database")
	}
	return path, cleanup, nil
}

func copySQLiteDatabase(ctx context.Context, destination, source *sql.DB) error {
	destinationConn, err := destination.Conn(ctx)
	if err != nil {
		return err
	}
	defer destinationConn.Close()
	sourceConn, err := source.Conn(ctx)
	if err != nil {
		return err
	}
	defer sourceConn.Close()

	return destinationConn.Raw(func(destinationDriver any) error {
		dest, ok := destinationDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("destination is not a SQLite connection")
		}
		return sourceConn.Raw(func(sourceDriver any) error {
			src, ok := sourceDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("source is not a SQLite connection")
			}
			backup, err := dest.Backup("main", src, "main")
			if err != nil {
				return err
			}
			for {
				done, stepErr := backup.Step(-1)
				if stepErr != nil {
					_ = backup.Finish()
					return stepErr
				}
				if done {
					return backup.Finish()
				}
				select {
				case <-ctx.Done():
					_ = backup.Finish()
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
	})
}

func openBackupDatabase(ctx context.Context, raw []byte) (*sql.DB, func(), error) {
	if !bytes.HasPrefix(raw, []byte(sqliteHeader)) {
		return nil, nil, fmt.Errorf("backup must be a TapX SQLite .db file")
	}
	dir, err := os.MkdirTemp("", "tapx-db-restore-")
	if err != nil {
		return nil, nil, err
	}
	cleanupDir := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "tapx.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		cleanupDir()
		return nil, nil, err
	}
	db, closeDB, err := openBackupDatabasePath(ctx, path)
	if err != nil {
		cleanupDir()
		return nil, nil, err
	}
	cleanup := func() {
		closeDB()
		cleanupDir()
	}
	return db, cleanup, nil
}

func openBackupDatabasePath(ctx context.Context, path string) (*sql.DB, func(), error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	cleanup := func() { _ = db.Close() }
	if err := validateBackupDatabase(ctx, db); err != nil {
		cleanup()
		return nil, nil, err
	}
	return db, cleanup, nil
}

func validateBackupDatabase(ctx context.Context, db *sql.DB) error {
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("SQLite integrity check: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("SQLite integrity check failed: %s", integrity)
	}
	for _, table := range []string{"tapx_objects", "tapx_integrations", "tapx_logs", "tapx_metrics"} {
		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("backup is missing required table %s", table)
		}
	}
	return validateStoreConfig(ctx, db)
}

func validateStoreConfig(ctx context.Context, db *sql.DB) error {
	temporary := &Store{db: db, dialect: DatabaseSQLite}
	cfg, err := temporary.LoadConfig(ctx)
	if err != nil {
		return err
	}
	if err := config.ValidateForSave(cfg); err != nil {
		return fmt.Errorf("invalid TapX configuration: %w", err)
	}
	return nil
}
