package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppPort         string
	APIKey          string
	DB              DBConfig
	RDB             RDBConfig
	EventServiceURL string
}

type RDBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		d.Host, d.User, d.Password, d.Name, d.Port)
}

func Load() *Config {
	return &Config{
		AppPort:         getEnv("SERVER_PORT", "8082"),
		APIKey:          getEnv("API_KEY", ""),
		EventServiceURL: getEnv("EVENT_SERVICE_URL", "http://localhost:8081"),
		DB: DBConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     getEnv("POSTGRES_USER", "eventbooking"),
			Password: getEnv("POSTGRES_PASSWORD", "eventbooking"),
			Name:     getEnv("POSTGRES_DB", "eventdb"),
		},
		RDB: RDBConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			User:     getEnv("REDIS_USER", ""),
			Password: getEnv("REDIS_PASSWORD", ""),
			Name:     getEnv("REDIS_DB", "0"),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
