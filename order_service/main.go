package main

import (
	"log"

	"order_service/config"
	"order_service/internal/api"
	"order_service/internal/api/rest/handler"
	"order_service/internal/client"
	"order_service/internal/entity"
	"order_service/internal/helper"
	"order_service/internal/repository"
	"order_service/internal/service"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// ─── Step 1: Load Configuration ──────────────────────────────────────────
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// ─── Step 2: Connect to PostgreSQL Database ─────────────────────────────
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// ─── Step 3: Run Auto-Migrations ────────────────────────────────────────
	if err := db.AutoMigrate(&entity.Order{}, &entity.OrderItem{}); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// ─── Step 4: Initialize Product Service gRPC Client ─────────────────────
	productClient, err := client.NewProductGRPCClient(cfg.ProductServiceGRPCAddr, cfg.ProductServiceTimeout)
	if err != nil {
		log.Fatalf("Failed to initialize Product Service gRPC client: %v", err)
	}

	// ─── Step 5: Initialize Layers (Repo -> Service -> Handler) ─────────────
	orderRepo := repository.NewOrderRepository(db)
	orderService := service.NewOrderService(orderRepo, productClient)
	orderHandler := handler.NewOrderHandler(orderService)
	tokenHelper := helper.NewTokenHelper(cfg.JWTSecret)

	// ─── Step 6: Initialize Combined Server Manager ─────────────────────────
	server := api.NewServer(cfg, db, orderHandler, tokenHelper, productClient)

	// ─── Step 7: Start Server with Graceful Shutdown ────────────────────────
	if err := server.Start(); err != nil {
		log.Fatalf("Order Service server error: %v", err)
	}
}

