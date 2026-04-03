package database

import (
	"booking-service/config"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewPostgres(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DB.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres db handle: %w", err)
	}

	sqlDB.SetMaxOpenConns(300)
	sqlDB.SetMaxIdleConns(80)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	return db, nil
}
