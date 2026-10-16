package common

import (
	"os"
	"path/filepath"
	"sync"
)

type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

var mainDatabaseType = DatabaseTypeSQLite
var logDatabaseType = DatabaseTypeSQLite

func MainDatabaseType() DatabaseType {
	return mainDatabaseType
}

func LogDatabaseType() DatabaseType {
	return logDatabaseType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	mainDatabaseType = databaseType
}

func SetLogDatabaseType(databaseType DatabaseType) {
	logDatabaseType = databaseType
}

func SetDatabaseTypes(mainType DatabaseType, logType DatabaseType) {
	mainDatabaseType = mainType
	logDatabaseType = logType
}

func UsingMainDatabase(databaseType DatabaseType) bool {
	return mainDatabaseType == databaseType
}

func UsingLogDatabase(databaseType DatabaseType) bool {
	return logDatabaseType == databaseType
}

// DSNConfigPath returns the path to the persisted DSN config file.
func DSNConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ".newapi-dsn"
	}
	return filepath.Join(filepath.Dir(exe), ".newapi-dsn")
}

// ReadDSNConfig reads the persisted DSN from the config file.
// Returns empty string if the file doesn't exist or is unreadable.
var readDSNOnce sync.Once
var cachedDSN string

// ReadDSNConfig reads the persisted DSN from the config file.
// Results are cached after the first read.
func ReadDSNConfig() string {
	readDSNOnce.Do(func() {
		data, err := os.ReadFile(DSNConfigPath())
		if err == nil {
			cachedDSN = string(data)
		}
	})
	return cachedDSN
}

// SaveDSNConfig persists the DSN to the config file.
func SaveDSNConfig(dsn string) error {
	return os.WriteFile(DSNConfigPath(), []byte(dsn), 0o644)
}

// ClearDSNConfig removes the persisted DSN config file.
func ClearDSNConfig() error {
	readDSNOnce = sync.Once{}
	cachedDSN = ""
	return os.Remove(DSNConfigPath())
}

// SQLitePath is the DSN for the default SQLite database. It uses WAL journal
// mode so readers are never blocked by the single writer, plus a 30s busy
// timeout for writers to queue.
//
// Two details are non-obvious and both are required for concurrent correctness:
//
//  1. The busy timeout must be passed as a `_pragma=busy_timeout(30000)` DSN
//     parameter. The pure-Go driver (modernc.org/sqlite, used through
//     github.com/glebarez/sqlite) silently ignores the plain `_busy_timeout=`
//     form, so without this the effective timeout stays at SQLite's 5s default
//     and concurrent writes surface as "database is locked" (see #6805).
//
//  2. `_txlock=immediate` (BEGIN IMMEDIATE) must be enabled. Without it, a
//     transaction that first SELECTs (establishing a read snapshot) and then
//     writes can hit SQLITE_BUSY_SNAPSHOT when another connection commits in
//     between; the busy handler does not cover that case, so the write fails
//     instantly no matter the timeout. BEGIN IMMEDIATE takes the write lock up
//     front, so writers serialize through the busy timeout instead of dying on
//     a stale snapshot. Autocommit SELECTs stay concurrent because WAL keeps
//     readers unlocked.
var SQLitePath = "one-api.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
