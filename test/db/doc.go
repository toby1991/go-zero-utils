// Package db provides isolated external database fixtures for Go tests.
//
// MySQL tests install one package-scoped runtime from TestMain:
//
//	func TestMain(m *testing.M) {
//		os.Exit(testdb.RunMySQL(m))
//	}
//
// Each test then obtains its own temporary database with NewMySQL. The package
// never falls back to a developer or production DSN.
package db
