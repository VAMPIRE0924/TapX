package panel

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"tapx/internal/config"
)

func TestPostgresStoreAndPortableBackupRoundTrip(t *testing.T) {
	rawDSN := os.Getenv("TAPX_TEST_POSTGRES_DSN")
	if rawDSN == "" {
		t.Skip("TAPX_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	admin, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatalf("open postgres admin connection: %v", err)
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	schema := fmt.Sprintf("tapx_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatalf("create postgres test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
	})

	dsn := postgresDSNWithSearchPath(t, rawDSN, schema)
	store, err := OpenStoreWithOptions(StoreOptions{Driver: DatabasePostgres, DataSource: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer store.Close()
	if store.DatabaseDriver() != DatabasePostgres {
		t.Fatalf("database driver = %q", store.DatabaseDriver())
	}

	cfg := sampleConfig()
	if err := store.ReplaceConfig(ctx, cfg); err != nil {
		t.Fatalf("replace postgres config: %v", err)
	}
	if err := store.SetIntegration(ctx, "warp", map[string]string{"device": "test"}); err != nil {
		t.Fatalf("store postgres integration: %v", err)
	}
	if err := store.AppendLog(ctx, LogEvent{Seq: 42, Time: "2026-07-16T00:00:00Z", Level: "info", Action: "postgres", Message: "round trip"}, 10); err != nil {
		t.Fatalf("store postgres log: %v", err)
	}
	metric := DashboardMetricSample{At: 1, CPU: 12.5, Memory: 25, TapX: 1, RX: 100, TX: 200, DiskRead: 300, Load1: 0.5, TapXHeap: 500, Drops: 3}
	if err := store.AppendMetric(ctx, metric, 0, 10); err != nil {
		t.Fatalf("store postgres metric: %v", err)
	}

	backup, err := store.BackupDatabase(ctx)
	if err != nil {
		t.Fatalf("backup postgres store: %v", err)
	}
	if !bytes.HasPrefix(backup, []byte(sqliteHeader)) {
		t.Fatalf("postgres backup is not a portable TapX .db")
	}
	sqliteStore, err := OpenStore(filepath.Join(t.TempDir(), "postgres-export.db"))
	if err != nil {
		t.Fatalf("open SQLite migration target: %v", err)
	}
	defer sqliteStore.Close()
	if err := sqliteStore.RestoreDatabase(ctx, backup); err != nil {
		t.Fatalf("restore PostgreSQL backup into SQLite: %v", err)
	}
	if migrated, err := sqliteStore.LoadConfig(ctx); err != nil || len(migrated.Devices) != 1 || migrated.Devices[0].ID != "tun-a" {
		t.Fatalf("PostgreSQL to SQLite migration = %+v, err=%v", migrated, err)
	}
	if err := sqliteStore.SetIntegration(ctx, "warp", map[string]string{"device": "from-sqlite"}); err != nil {
		t.Fatalf("update migrated SQLite integration: %v", err)
	}
	sqliteBackup, err := sqliteStore.BackupDatabase(ctx)
	if err != nil {
		t.Fatalf("backup migrated SQLite store: %v", err)
	}
	if err := store.ReplaceConfigAndIntegrations(ctx, sampleConfigWithoutObjects(), nil, nil, nil); err != nil {
		t.Fatalf("clear postgres store: %v", err)
	}
	if err := store.RestoreDatabase(ctx, sqliteBackup); err != nil {
		t.Fatalf("restore SQLite backup into PostgreSQL: %v", err)
	}

	loaded, err := store.LoadConfig(ctx)
	if err != nil || len(loaded.Devices) != 1 || loaded.Devices[0].ID != "tun-a" {
		t.Fatalf("restored postgres config = %+v, err=%v", loaded, err)
	}
	if integration, err := store.GetIntegration(ctx, "warp"); err != nil || !bytes.Contains(integration, []byte(`"device":"from-sqlite"`)) {
		t.Fatalf("restored postgres integration = %s, err=%v", integration, err)
	}
	if logs, err := store.LoadLogs(ctx, 10); err != nil || len(logs) != 1 || logs[0].Seq != 42 || logs[0].Action != "postgres" {
		t.Fatalf("restored postgres logs = %+v, err=%v", logs, err)
	}
	if metrics, err := store.LoadMetrics(ctx, 10); err != nil || len(metrics) != 1 || !reflect.DeepEqual(metrics[0], metric) {
		t.Fatalf("restored postgres metrics = %+v, err=%v", metrics, err)
	}
}

func postgresDSNWithSearchPath(t *testing.T, rawDSN, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawDSN)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatalf("TAPX_TEST_POSTGRES_DSN must be a postgres:// or postgresql:// URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sampleConfigWithoutObjects() config.RuntimeConfig {
	return config.RuntimeConfig{}
}
