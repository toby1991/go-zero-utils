package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	testcontainersMySQL "github.com/testcontainers/testcontainers-go/modules/mysql"
)

const (
	defaultMySQLImage            = "mysql:8.4.6"
	defaultMySQLDatabasePrefix   = "go_zero_test_"
	defaultMySQLBootstrapDB      = "go_zero_test_bootstrap"
	defaultMySQLUsername         = "go_zero_test"
	defaultMySQLObserverDatabase = "performance_schema"
	mysqlStartTimeout            = 3 * time.Minute
	mysqlOperationTimeout        = 30 * time.Second
)

var (
	defaultMySQLSQLModes = []string{
		"ONLY_FULL_GROUP_BY",
		"STRICT_TRANS_TABLES",
		"ERROR_FOR_DIVISION_BY_ZERO",
		"NO_ENGINE_SUBSTITUTION",
	}
	validMySQLDatabaseName = regexp.MustCompile(`^go_zero_test_[0-9a-f]{32}$`)
	validMySQLSQLMode      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

	activeMySQLRuntime = struct {
		sync.Mutex
		manager *mysqlManager
	}{}
)

// MySQLOption configures the package-scoped MySQL test runtime.
type MySQLOption interface {
	applyMySQL(*mysqlConfig) error
}

// MySQLDatabase is one temporary database owned by one testing.TB.
type MySQLDatabase struct {
	name        string
	dsn         string
	observerDSN string
}

// Name returns the isolated database name.
func (d *MySQLDatabase) Name() string {
	return d.name
}

// DSN returns the normal test-user DSN for the isolated database.
func (d *MySQLDatabase) DSN() string {
	return d.dsn
}

// ObserverDSN returns a root DSN for performance_schema.
//
// It exists for lock and transaction observation tests. Business reads and
// writes should use DSN instead.
func (d *MySQLDatabase) ObserverDSN() string {
	return d.observerDSN
}

// WithMySQLImage overrides the default mysql:8.4.6 container image.
func WithMySQLImage(image string) MySQLOption {
	return mysqlOptionFunc(func(config *mysqlConfig) error {
		image = strings.TrimSpace(image)
		if image == "" {
			return errors.New("MySQL image must not be empty")
		}

		config.image = image
		return nil
	})
}

// WithMySQLSQLMode overrides the SQL Mode passed to the MySQL server.
func WithMySQLSQLMode(modes ...string) MySQLOption {
	modes = append([]string(nil), modes...)

	return mysqlOptionFunc(func(config *mysqlConfig) error {
		normalizedModes, err := normalizeMySQLSQLModes(modes)
		if err != nil {
			return err
		}

		config.sqlModes = normalizedModes
		return nil
	})
}

// RunMySQL installs a package-scoped, lazily started MySQL runtime around m.Run.
//
// Call it exactly once from TestMain with os.Exit(testdb.RunMySQL(m)). If no
// test calls NewMySQL, no container is started.
func RunMySQL(m *testing.M, options ...MySQLOption) (exitCode int) {
	if m == nil {
		fmt.Fprintln(os.Stderr, "test/db: RunMySQL received a nil testing.M")
		return 1
	}

	config, err := buildMySQLConfig(options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test/db: configure MySQL runtime: %v\n", err)
		return 1
	}

	manager := newMySQLManager(config)
	if err := installMySQLManager(manager); err != nil {
		fmt.Fprintf(os.Stderr, "test/db: install MySQL runtime: %v\n", err)
		return 1
	}

	// m.Run 返回后所有 testing.TB cleanup 已执行；此时兜底清理残留库并停止容器。
	defer func() {
		cleanupErr := manager.close()
		uninstallMySQLManager(manager)
		if cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "test/db: close MySQL runtime: %v\n", cleanupErr)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}()

	exitCode = m.Run()
	return exitCode
}

// NewMySQL returns the isolated MySQL database owned by t.
//
// Repeated calls with the same testing.TB return the same database. Calls from
// different tests or subtests always receive different databases.
func NewMySQL(t testing.TB) *MySQLDatabase {
	t.Helper()

	manager, err := installedMySQLManager()
	if err != nil {
		t.Fatalf("test/db: NewMySQL: %v", err)
	}

	database, err := manager.databaseFor(t)
	if err != nil {
		t.Fatalf("test/db: NewMySQL: %v", err)
	}

	return database
}

type mysqlOptionFunc func(*mysqlConfig) error

func (f mysqlOptionFunc) applyMySQL(config *mysqlConfig) error {
	if f == nil {
		return errors.New("nil MySQL option")
	}

	return f(config)
}

type mysqlConfig struct {
	image    string
	sqlModes []string
}

func buildMySQLConfig(options []MySQLOption) (mysqlConfig, error) {
	config := mysqlConfig{
		image:    defaultMySQLImage,
		sqlModes: append([]string(nil), defaultMySQLSQLModes...),
	}

	for index, option := range options {
		if option == nil {
			return mysqlConfig{}, fmt.Errorf("option %d is nil", index)
		}
		if err := option.applyMySQL(&config); err != nil {
			return mysqlConfig{}, fmt.Errorf("option %d: %w", index, err)
		}
	}

	return config, nil
}

func normalizeMySQLSQLModes(modes []string) ([]string, error) {
	if len(modes) == 0 {
		return nil, errors.New("MySQL SQL Mode list must not be empty")
	}

	normalizedModes := make([]string, 0, len(modes))
	seen := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		normalizedMode := strings.ToUpper(strings.TrimSpace(mode))
		if !validMySQLSQLMode.MatchString(normalizedMode) {
			return nil, fmt.Errorf("invalid MySQL SQL Mode %q", mode)
		}
		if _, duplicated := seen[normalizedMode]; duplicated {
			continue
		}

		seen[normalizedMode] = struct{}{}
		normalizedModes = append(normalizedModes, normalizedMode)
	}

	return normalizedModes, nil
}

type mysqlManager struct {
	mu sync.Mutex

	config mysqlConfig
	closed bool

	container *testcontainersMySQL.MySQLContainer
	adminDB   *sql.DB
	address   string
	password  string

	databasesByTest map[testing.TB]*MySQLDatabase
	createdDatabase map[string]struct{}
}

func newMySQLManager(config mysqlConfig) *mysqlManager {
	return &mysqlManager{
		config:          config,
		databasesByTest: make(map[testing.TB]*MySQLDatabase),
		createdDatabase: make(map[string]struct{}),
	}
}

func installMySQLManager(manager *mysqlManager) error {
	activeMySQLRuntime.Lock()
	defer activeMySQLRuntime.Unlock()

	if activeMySQLRuntime.manager != nil {
		return errors.New("RunMySQL is already active in this test process")
	}

	activeMySQLRuntime.manager = manager
	return nil
}

func uninstallMySQLManager(manager *mysqlManager) {
	activeMySQLRuntime.Lock()
	defer activeMySQLRuntime.Unlock()

	if activeMySQLRuntime.manager == manager {
		activeMySQLRuntime.manager = nil
	}
}

func installedMySQLManager() (*mysqlManager, error) {
	activeMySQLRuntime.Lock()
	defer activeMySQLRuntime.Unlock()

	if activeMySQLRuntime.manager == nil {
		return nil, errors.New("RunMySQL is not installed; call os.Exit(testdb.RunMySQL(m)) from TestMain")
	}

	return activeMySQLRuntime.manager, nil
}

func (m *mysqlManager) databaseFor(t testing.TB) (*MySQLDatabase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("MySQL runtime is closed")
	}
	if database := m.databasesByTest[t]; database != nil {
		return database, nil
	}
	if err := m.startLocked(); err != nil {
		return nil, err
	}

	databaseName, err := newMySQLDatabaseName()
	if err != nil {
		return nil, err
	}
	if err := m.createDatabaseLocked(databaseName); err != nil {
		return nil, err
	}

	database := &MySQLDatabase{
		name:        databaseName,
		dsn:         formatMySQLDSN(defaultMySQLUsername, m.password, m.address, databaseName),
		observerDSN: formatMySQLDSN("root", m.password, m.address, defaultMySQLObserverDatabase),
	}
	m.databasesByTest[t] = database
	m.createdDatabase[databaseName] = struct{}{}

	// 每个 testing.TB 只注册一次 cleanup；删除前仍会校验本 manager 的所有权。
	t.Cleanup(func() {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), mysqlOperationTimeout)
		defer cancel()

		if err := m.dropDatabase(ctx, t, databaseName); err != nil {
			t.Errorf("test/db: drop temporary MySQL database %q: %v", databaseName, err)
		}
	})

	return database, nil
}

func (m *mysqlManager) startLocked() error {
	if m.container != nil {
		return nil
	}

	password, err := newRandomHex(24)
	if err != nil {
		return fmt.Errorf("generate MySQL test password: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mysqlStartTimeout)
	defer cancel()

	container, err := testcontainersMySQL.Run(
		ctx,
		m.config.image,
		testcontainersMySQL.WithDatabase(defaultMySQLBootstrapDB),
		testcontainersMySQL.WithUsername(defaultMySQLUsername),
		testcontainersMySQL.WithPassword(password),
		withMySQLServerConfig(m.config.sqlModes),
	)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return fmt.Errorf("start MySQL container %q: %w", m.config.image, err)
	}

	connectionString, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return fmt.Errorf("read MySQL container connection string: %w", err)
	}
	connectionConfig, err := mysqlDriver.ParseDSN(connectionString)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return fmt.Errorf("parse MySQL container connection string: %w", err)
	}

	adminDB, err := sql.Open(
		"mysql",
		formatMySQLDSN("root", password, connectionConfig.Addr, ""),
	)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return fmt.Errorf("open MySQL admin connection: %w", err)
	}
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		_ = testcontainers.TerminateContainer(container)
		return fmt.Errorf("ping MySQL admin connection: %w", err)
	}

	m.container = container
	m.adminDB = adminDB
	m.address = connectionConfig.Addr
	m.password = password

	return nil
}

func withMySQLServerConfig(sqlModes []string) testcontainers.CustomizeRequestOption {
	sqlModes = append([]string(nil), sqlModes...)

	return func(request *testcontainers.GenericContainerRequest) error {
		request.Cmd = append(
			request.Cmd,
			"--character-set-server=utf8mb4",
			"--sql-mode="+strings.Join(sqlModes, ","),
		)
		return nil
	}
}

func (m *mysqlManager) createDatabaseLocked(databaseName string) error {
	if !validMySQLDatabaseName.MatchString(databaseName) {
		return fmt.Errorf("unsafe generated MySQL database name %q", databaseName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mysqlOperationTimeout)
	defer cancel()

	if _, err := m.adminDB.ExecContext(
		ctx,
		"CREATE DATABASE "+quoteMySQLIdentifier(databaseName)+" CHARACTER SET utf8mb4",
	); err != nil {
		return fmt.Errorf("create temporary MySQL database %q: %w", databaseName, err)
	}
	if _, err := m.adminDB.ExecContext(
		ctx,
		"GRANT ALL PRIVILEGES ON "+quoteMySQLIdentifier(databaseName)+".* TO '"+defaultMySQLUsername+"'@'%'",
	); err != nil {
		_, _ = m.adminDB.ExecContext(ctx, "DROP DATABASE "+quoteMySQLIdentifier(databaseName))
		return fmt.Errorf("grant temporary MySQL database %q: %w", databaseName, err)
	}

	return nil
}

func (m *mysqlManager) dropDatabase(
	ctx context.Context,
	owner testing.TB,
	databaseName string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	database := m.databasesByTest[owner]
	if database == nil || database.name != databaseName {
		return fmt.Errorf("database %q is not owned by this test", databaseName)
	}
	if _, created := m.createdDatabase[databaseName]; !created {
		return fmt.Errorf("database %q was not created by this MySQL runtime", databaseName)
	}
	if !validMySQLDatabaseName.MatchString(databaseName) {
		return fmt.Errorf("refuse to drop unsafe MySQL database name %q", databaseName)
	}
	if m.adminDB == nil {
		return errors.New("MySQL admin connection is not available")
	}

	if _, err := m.adminDB.ExecContext(
		ctx,
		"DROP DATABASE "+quoteMySQLIdentifier(databaseName),
	); err != nil {
		return err
	}

	delete(m.databasesByTest, owner)
	delete(m.createdDatabase, databaseName)
	return nil
}

func (m *mysqlManager) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	var closeErrors []error
	if m.adminDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), mysqlOperationTimeout)
		for databaseName := range m.createdDatabase {
			if !validMySQLDatabaseName.MatchString(databaseName) {
				closeErrors = append(closeErrors, fmt.Errorf("refuse to drop unsafe MySQL database name %q", databaseName))
				continue
			}
			if _, err := m.adminDB.ExecContext(
				ctx,
				"DROP DATABASE IF EXISTS "+quoteMySQLIdentifier(databaseName),
			); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("drop temporary MySQL database %q: %w", databaseName, err))
			}
		}
		cancel()

		if err := m.adminDB.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close MySQL admin connection: %w", err))
		}
	}
	if err := testcontainers.TerminateContainer(m.container); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("terminate MySQL container: %w", err))
	}

	return errors.Join(closeErrors...)
}

func newMySQLDatabaseName() (string, error) {
	suffix, err := newRandomHex(16)
	if err != nil {
		return "", fmt.Errorf("generate temporary MySQL database name: %w", err)
	}

	return defaultMySQLDatabasePrefix + suffix, nil
}

func newRandomHex(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}

	return hex.EncodeToString(value), nil
}

func formatMySQLDSN(user, password, address, databaseName string) string {
	config := mysqlDriver.NewConfig()
	config.User = user
	config.Passwd = password
	config.Net = "tcp"
	config.Addr = address
	config.DBName = databaseName
	config.ParseTime = true
	config.Params = map[string]string{
		"charset": "utf8mb4",
	}

	return config.FormatDSN()
}

func quoteMySQLIdentifier(identifier string) string {
	return "`" + identifier + "`"
}
