package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppPort string
	DB      DBConfig
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable search_path=public",
		d.Host, d.User, d.Password, d.Name, d.Port)
}

func Load() *Config {
	return &Config{
		AppPort: getEnv("SERVER_PORT", "8081"),
		DB: DBConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     getEnv("POSTGRES_USER", "eventbooking"),
			Password: getEnv("POSTGRES_PASSWORD", "eventbooking"),
			Name:     getEnv("POSTGRES_DB", "eventdb"),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
