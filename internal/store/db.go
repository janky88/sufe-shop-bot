package store

import (
	"fmt"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	
	// Determine database type from DSN
	if strings.HasPrefix(dsn, "file:") || dsn == ":memory:" {
		dialector = sqlite.Open(dsn)
	} else {
		dialector = postgres.Open(dsn)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	// Run migrations
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	DB = db
	return db, nil
}

// AutoMigrate creates/updates database schema
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&Product{},
		&Code{},
		&Order{},
		&RechargeCard{},
		&BalanceTransaction{},
		&MessageTemplate{},
		&Group{},
		&GroupAdmin{},
		&BroadcastMessage{},
		&BroadcastLog{},
	)
}

// IsPostgres checks if the database is PostgreSQL
func IsPostgres(db *gorm.DB) bool {
	return db.Dialector.Name() == "postgres"
}