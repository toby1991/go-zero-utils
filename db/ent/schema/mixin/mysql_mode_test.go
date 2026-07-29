package mixin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestMySQLTimeMixinCompatibilityCompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		compatibility MySQLTimeMixinCompatibility
		want          bool
	}{
		{
			name: "compatible",
			compatibility: MySQLTimeMixinCompatibility{
				GlobalSQLMode:  "STRICT_TRANS_TABLES",
				SessionSQLMode: "STRICT_TRANS_TABLES",
			},
			want: true,
		},
		{
			name: "incompatible global mode",
			compatibility: MySQLTimeMixinCompatibility{
				IncompatibleGlobalModes: []string{"NO_ZERO_DATE"},
			},
			want: false,
		},
		{
			name: "incompatible session mode",
			compatibility: MySQLTimeMixinCompatibility{
				IncompatibleSessionModes: []string{"TRADITIONAL"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.compatibility.Compatible(); got != tt.want {
				t.Fatalf("Compatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindIncompatibleMySQLTimeMixinModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sqlMode string
		want    []string
	}{
		{
			name:    "compatible compose modes",
			sqlMode: "ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION",
			want:    nil,
		},
		{
			name:    "exact tokens are case insensitive",
			sqlMode: " no_zero_date,Traditional ",
			want:    []string{"NO_ZERO_DATE", "TRADITIONAL"},
		},
		{
			name:    "similar token is not a match",
			sqlMode: "SOME_NO_ZERO_DATE_MODE,NO_ZERO_IN_DATE",
			want:    nil,
		},
		{
			name:    "duplicates are removed",
			sqlMode: "NO_ZERO_DATE,no_zero_date",
			want:    []string{"NO_ZERO_DATE"},
		},
		{
			name:    "empty mode",
			sqlMode: "",
			want:    nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := findIncompatibleMySQLTimeMixinModes(tt.sqlMode); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findIncompatibleMySQLTimeMixinModes(%q) = %#v, want %#v", tt.sqlMode, got, tt.want)
			}
		})
	}
}

func TestInspectMySQLTimeMixinCompatibility(t *testing.T) {
	t.Parallel()

	database := sql.OpenDB(mysqlModeConnector{
		rows: [][]driver.Value{{
			"STRICT_TRANS_TABLES,NO_ZERO_DATE",
			"STRICT_TRANS_TABLES",
		}},
	})
	t.Cleanup(func() {
		_ = database.Close()
	})

	got, err := InspectMySQLTimeMixinCompatibility(context.Background(), database)
	if err != nil {
		t.Fatalf("InspectMySQLTimeMixinCompatibility() error = %v", err)
	}

	want := MySQLTimeMixinCompatibility{
		GlobalSQLMode:            "STRICT_TRANS_TABLES,NO_ZERO_DATE",
		SessionSQLMode:           "STRICT_TRANS_TABLES",
		IncompatibleGlobalModes:  []string{"NO_ZERO_DATE"},
		IncompatibleSessionModes: nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("InspectMySQLTimeMixinCompatibility() = %#v, want %#v", got, want)
	}
}

func TestInspectMySQLTimeMixinCompatibilityErrors(t *testing.T) {
	t.Parallel()

	t.Run("nil database", func(t *testing.T) {
		t.Parallel()

		if _, err := InspectMySQLTimeMixinCompatibility(context.Background(), nil); err == nil {
			t.Fatal("InspectMySQLTimeMixinCompatibility() error = nil, want non-nil")
		}
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("query failed")
		database := sql.OpenDB(mysqlModeConnector{err: wantErr})
		t.Cleanup(func() {
			_ = database.Close()
		})

		_, err := InspectMySQLTimeMixinCompatibility(context.Background(), database)
		if !errors.Is(err, wantErr) {
			t.Fatalf("InspectMySQLTimeMixinCompatibility() error = %v, want %v", err, wantErr)
		}
	})
}

type mysqlModeConnector struct {
	rows [][]driver.Value
	err  error
}

func (c mysqlModeConnector) Connect(context.Context) (driver.Conn, error) {
	return mysqlModeConnection(c), nil
}

func (mysqlModeConnector) Driver() driver.Driver {
	return mysqlModeDriver{}
}

type mysqlModeDriver struct{}

func (mysqlModeDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("mysqlModeDriver.Open is not supported")
}

type mysqlModeConnection mysqlModeConnector

func (mysqlModeConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("mysqlModeConnection.Prepare is not supported")
}

func (mysqlModeConnection) Close() error {
	return nil
}

func (mysqlModeConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("mysqlModeConnection.Begin is not supported")
}

func (c mysqlModeConnection) QueryContext(
	_ context.Context,
	query string,
	_ []driver.NamedValue,
) (driver.Rows, error) {
	if query != inspectMySQLTimeMixinCompatibilityQuery {
		return nil, errors.New("unexpected query")
	}
	if c.err != nil {
		return nil, c.err
	}

	return &mysqlModeRows{rows: c.rows}, nil
}

type mysqlModeRows struct {
	rows  [][]driver.Value
	index int
}

func (*mysqlModeRows) Columns() []string {
	return []string{"@@GLOBAL.sql_mode", "@@SESSION.sql_mode"}
}

func (*mysqlModeRows) Close() error {
	return nil
}

func (r *mysqlModeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}

	copy(dest, r.rows[r.index])
	r.index++

	return nil
}
