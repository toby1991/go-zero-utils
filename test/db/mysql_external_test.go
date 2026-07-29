//go:build external_integration

package db

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestMain(m *testing.M) {
	os.Exit(RunMySQL(m))
}

func TestMySQLDatabaseExternalIntegration(t *testing.T) {
	database := NewMySQL(t)
	if got := NewMySQL(t); got != database {
		t.Fatalf("NewMySQL(t) returned %p after %p, want the same database", got, database)
	}

	client, err := sql.Open("mysql", database.DSN())
	if err != nil {
		t.Fatalf("sql.Open(database.DSN()) error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx := context.Background()
	if err := client.PingContext(ctx); err != nil {
		t.Fatalf("database PingContext() error = %v", err)
	}
	if _, err := client.ExecContext(ctx, `
		CREATE TABLE fixture (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			value VARCHAR(64) NOT NULL,
			PRIMARY KEY (id)
		)
	`); err != nil {
		t.Fatalf("create fixture table error = %v", err)
	}
	if _, err := client.ExecContext(ctx, `INSERT INTO fixture (value) VALUES (?)`, "ready"); err != nil {
		t.Fatalf("insert fixture row error = %v", err)
	}

	var value string
	if err := client.QueryRowContext(ctx, `SELECT value FROM fixture WHERE id = 1`).Scan(&value); err != nil {
		t.Fatalf("select fixture row error = %v", err)
	}
	if value != "ready" {
		t.Fatalf("fixture value = %q, want %q", value, "ready")
	}

	observer, err := sql.Open("mysql", database.ObserverDSN())
	if err != nil {
		t.Fatalf("sql.Open(database.ObserverDSN()) error = %v", err)
	}
	t.Cleanup(func() {
		_ = observer.Close()
	})
	if err := observer.PingContext(ctx); err != nil {
		t.Fatalf("observer PingContext() error = %v", err)
	}

	var observerDatabase string
	if err := observer.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&observerDatabase); err != nil {
		t.Fatalf("select observer database error = %v", err)
	}
	if observerDatabase != defaultMySQLObserverDatabase {
		t.Fatalf("observer database = %q, want %q", observerDatabase, defaultMySQLObserverDatabase)
	}

	var sessionSQLMode string
	if err := client.QueryRowContext(ctx, `SELECT @@SESSION.sql_mode`).Scan(&sessionSQLMode); err != nil {
		t.Fatalf("select session SQL Mode error = %v", err)
	}
	gotSQLModes := splitMySQLModes(sessionSQLMode)
	wantSQLModes := append([]string(nil), defaultMySQLSQLModes...)
	sort.Strings(gotSQLModes)
	sort.Strings(wantSQLModes)
	if !reflect.DeepEqual(gotSQLModes, wantSQLModes) {
		t.Fatalf("session SQL Mode = %#v, want %#v", gotSQLModes, wantSQLModes)
	}
}

func TestMySQLDatabaseIsolationAndCleanupExternalIntegration(t *testing.T) {
	var firstName string
	t.Run("first", func(t *testing.T) {
		firstName = NewMySQL(t).Name()
	})

	var secondName string
	t.Run("second", func(t *testing.T) {
		secondName = NewMySQL(t).Name()
	})

	if firstName == secondName {
		t.Fatalf("subtests received the same database %q", firstName)
	}

	manager, err := installedMySQLManager()
	if err != nil {
		t.Fatalf("installedMySQLManager() error = %v", err)
	}

	manager.mu.Lock()
	adminDB := manager.adminDB
	manager.mu.Unlock()

	for _, databaseName := range []string{firstName, secondName} {
		var count int
		if err := adminDB.QueryRow(
			`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?`,
			databaseName,
		).Scan(&count); err != nil {
			t.Fatalf("check database %q cleanup error = %v", databaseName, err)
		}
		if count != 0 {
			t.Fatalf("database %q remains after subtest cleanup", databaseName)
		}
	}
}

func splitMySQLModes(value string) []string {
	parts := strings.Split(value, ",")
	modes := make([]string, 0, len(parts))
	for _, part := range parts {
		mode := strings.TrimSpace(part)
		if mode != "" {
			modes = append(modes, mode)
		}
	}
	return modes
}
