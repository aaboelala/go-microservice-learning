package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config stores all application configuration loaded from environment variables.
type Config struct {
	ServerPort    string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	JWTSecret     string
	JWTExpiration time.Duration
}

// LoadConfig reads configuration settings from environment variables with fallback defaults.
func LoadConfig() *Config {
	if os.Getenv("ENV") == "dev" {
		if err := godotenv.Load(); err != nil {
			log.Println("error loading .env file:", err)
		}
	}
		return &Config{
			ServerPort:    getEnv("SERVER_PORT", "8080"),
			DBHost:        getEnv("DB_HOST", "localhost"),
			DBPort:        getEnv("DB_PORT", "5432"),
			DBUser:        getEnv("DB_USER", "postgres"),
			DBPassword:    getEnv("DB_PASSWORD", "postgres"),
			DBName:        getEnv("DB_NAME", "user_service"),
			DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
			JWTSecret:     getEnv("JWT_SECRET", "super-secret-jwt-key"),
			JWTExpiration: parseDuration(getEnv("JWT_EXPIRATION", "24h")),
		}
}

// DSN generates PostgreSQL connection string for GORM.
func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" port=" + c.DBPort +
		" sslmode=" + c.DBSSLMode +
		" TimeZone=UTC"
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}
