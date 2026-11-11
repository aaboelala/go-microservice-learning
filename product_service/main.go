package main

import (
	"log"

	"product_service/config"
	"product_service/internal/api"
	grpcapi "product_service/internal/api/grpc"
	"product_service/internal/api/rest/handler"
	"product_service/internal/entiti"
	"product_service/internal/help"
	"product_service/internal/repostory"
	"product_service/internal/service"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// ─── Step 1: Load Config ──────────────────────────────────────────────────
	// Read all environment variables (.env in DEV, OS env in PROD)
	// and build the Config struct (DSN, JWT secret, ports, etc.)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// ─── Step 2: Connect to Database ─────────────────────────────────────────
	// Open a GORM connection using the DSN from config.
	// GORM handles the connection pool internally.
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&entiti.Product{}); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// ─── Step 3: Initialise Repository ───────────────────────────────────────
	// Inject the *gorm.DB into the repository layer.
	// The repository is the ONLY layer that talks to the database.
	repo := repostory.NewRepostory(db)

	// ─── Step 4: Initialise Service ──────────────────────────────────────────
	// Inject the repository into the service layer.
	// The service contains all business logic and depends on the repo interface.
	svc := service.NewService(repo)

	// ─── Step 5: Initialise REST Handler ─────────────────────────────────────
	// Inject the service into the HTTP handler layer.
	// The handler translates HTTP requests into service calls and returns responses.
	userHandler := handler.NewHandler(svc)

	// ─── Step 6: Initialise Help (JWT Validator) ─────────────────────────────
	// Build the HelpMethod with the shared JWT secret from config.
	// This secret must match the one used by user_service to sign tokens.
	helpHandler := help.NewHelpMethod(cfg.JWTSecret)

	// ─── Step 7: Initialise gRPC Server ──────────────────────────────────────
	// Create the gRPC server and register the ProductServiceServer implementation.
	// Both REST and gRPC share the EXACT same service layer (svc).
	grpcServer := grpcapi.NewGRPCServer(svc)

	// ─── Step 8: Initialise Combined Server Manager ──────────────────────────
	// Inject config, db, REST handler, helpHandler, and gRPC server.
	// The server coordinates concurrent execution and unified graceful shutdown.
	server := api.NewServer(cfg, db, userHandler, helpHandler, grpcServer)

	// ─── Step 9: Start Servers ───────────────────────────────────────────────
	// Launches REST and gRPC servers concurrently on their respective ports.
	// Blocks until a termination signal or fatal server error occurs.
	if err := server.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
