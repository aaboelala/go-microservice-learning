package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env           string
	ServerPort    string
	GRPCPort      string
	DSN           string
	JWTSecret     string
	JWTExpiration time.Duration
}

func LoadConfig() (*Config, error) {
	if os.Getenv("ENV") == "DEV" {
		if err := godotenv.Load(); err != nil {
			log.Println("error to load ,env", err)
		}
	}

	Dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}

	config := &Config{
		Env:           os.Getenv("ENV"),
		ServerPort:    os.Getenv("SERVER_PORT"),
		GRPCPort:      grpcPort,
		DSN:           Dsn,
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTExpiration: time.Hour * 24, // Set JWT expiration to 24 hours
	}

	return config, nil

}
