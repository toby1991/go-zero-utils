package db

import (
	"reflect"
	"strings"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestBuildMySQLConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options []MySQLOption
		want    mysqlConfig
		wantErr bool
	}{
		{
			name: "defaults",
			want: mysqlConfig{
				image:    defaultMySQLImage,
				sqlModes: defaultMySQLSQLModes,
			},
		},
		{
			name: "overrides",
			options: []MySQLOption{
				WithMySQLImage(" mysql:8.0.36 "),
				WithMySQLSQLMode("strict_trans_tables", "NO_ENGINE_SUBSTITUTION", "strict_trans_tables"),
			},
			want: mysqlConfig{
				image:    "mysql:8.0.36",
				sqlModes: []string{"STRICT_TRANS_TABLES", "NO_ENGINE_SUBSTITUTION"},
			},
		},
		{
			name:    "nil option",
			options: []MySQLOption{nil},
			wantErr: true,
		},
		{
			name:    "empty image",
			options: []MySQLOption{WithMySQLImage(" ")},
			wantErr: true,
		},
		{
			name:    "empty SQL Mode list",
			options: []MySQLOption{WithMySQLSQLMode()},
			wantErr: true,
		},
		{
			name:    "invalid SQL Mode",
			options: []MySQLOption{WithMySQLSQLMode("STRICT_TRANS_TABLES,NO_ZERO_DATE")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildMySQLConfig(tt.options)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildMySQLConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildMySQLConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNewMySQLDatabaseName(t *testing.T) {
	t.Parallel()

	first, err := newMySQLDatabaseName()
	if err != nil {
		t.Fatalf("newMySQLDatabaseName() first error = %v", err)
	}
	second, err := newMySQLDatabaseName()
	if err != nil {
		t.Fatalf("newMySQLDatabaseName() second error = %v", err)
	}

	if first == second {
		t.Fatalf("newMySQLDatabaseName() generated duplicate name %q", first)
	}
	if !validMySQLDatabaseName.MatchString(first) {
		t.Fatalf("newMySQLDatabaseName() = %q, want strict safe prefix", first)
	}
	if !validMySQLDatabaseName.MatchString(second) {
		t.Fatalf("newMySQLDatabaseName() = %q, want strict safe prefix", second)
	}
	if validMySQLDatabaseName.MatchString("production_database") {
		t.Fatal("validMySQLDatabaseName accepts an arbitrary business database")
	}
}

func TestFormatMySQLDSN(t *testing.T) {
	t.Parallel()

	dsn := formatMySQLDSN("test_user", "secret", "127.0.0.1:3306", "test_database")
	config, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q) error = %v", dsn, err)
	}

	if config.User != "test_user" {
		t.Fatalf("DSN user = %q, want %q", config.User, "test_user")
	}
	if config.Passwd != "secret" {
		t.Fatalf("DSN password = %q, want %q", config.Passwd, "secret")
	}
	if config.Addr != "127.0.0.1:3306" {
		t.Fatalf("DSN address = %q, want %q", config.Addr, "127.0.0.1:3306")
	}
	if config.DBName != "test_database" {
		t.Fatalf("DSN database = %q, want %q", config.DBName, "test_database")
	}
	if !config.ParseTime {
		t.Fatal("DSN ParseTime = false, want true")
	}
	if !strings.Contains(dsn, "charset=utf8mb4") {
		t.Fatalf("DSN = %q, want charset=utf8mb4", dsn)
	}
}
