// Package connect creates the monolith's GORM database connection.
package connect

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(ctx context.Context, databaseURL string) (*gorm.DB, *sql.DB, error) {
	database, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	sqlDatabase, err := database.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get database handle: %w", err)
	}
	sqlDatabase.SetMaxOpenConns(10)
	sqlDatabase.SetMaxIdleConns(5)
	sqlDatabase.SetConnMaxLifetime(30 * time.Minute)

	if err := sqlDatabase.PingContext(ctx); err != nil {
		_ = sqlDatabase.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}
	return database, sqlDatabase, nil
}
