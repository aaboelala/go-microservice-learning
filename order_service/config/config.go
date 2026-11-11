package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the Order Service.
type Config struct {
	Env                    string
	ServerPort             string
	DSN                    string
	JWTSecret              string
	JWTExpiration          time.Duration
	ProductServiceGRPCAddr string
	ProductServiceTimeout  time.Duration
}

// LoadConfig loads environment variables from a .env file (in DEV mode) or OS environment.
func LoadConfig() (*Config, error) {
	env := os.Getenv("ENV")
	if env == "" {
		env = "DEV"
	}

	if env == "DEV" {
		if err := godotenv.Load(); err != nil {
			log.Println("Note: no .env file loaded or error loading .env:", err)
		}
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8082"
	}

	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "127.0.0.1"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "admin"
	}
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		dbPassword = "password"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "order_db"
	}
	dbSSLMode := os.Getenv("DB_SSLMODE")
	if dbSSLMode == "" {
		dbSSLMode = "disable"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode,
	)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-key-do-not-use-in-production"
	}

	productGRPCAddr := os.Getenv("PRODUCT_SERVICE_GRPC_ADDR")
	if productGRPCAddr == "" {
		productGRPCAddr = "127.0.0.1:50051"
	}

	timeoutSec := 3
	if secStr := os.Getenv("PRODUCT_SERVICE_TIMEOUT_SECONDS"); secStr != "" {
		if sec, err := strconv.Atoi(secStr); err == nil && sec > 0 {
			timeoutSec = sec
		}
	}

	cfg := &Config{
		Env:                    env,
		ServerPort:             serverPort,
		DSN:                    dsn,
		JWTSecret:              jwtSecret,
		JWTExpiration:          24 * time.Hour,
		ProductServiceGRPCAddr: productGRPCAddr,
		ProductServiceTimeout:  time.Duration(timeoutSec) * time.Second,
	}

	return cfg, nil
}

