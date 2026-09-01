// Package testdatabase provides an isolated GORM database for repository-backed tests.
package testdatabase

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var databaseSequence atomic.Uint64

func Open(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:test-%d?mode=memory&cache=shared", databaseSequence.Add(1))
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("get test database connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	return database
}
