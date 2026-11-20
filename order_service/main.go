package main

import (
	"log"

	"order_service/config"
	"order_service/internal/api"
	"order_service/internal/api/rest/handler"
	"order_service/internal/client"
	"order_service/internal/entity"
	"order_service/internal/helper"
	"order_service/internal/messaging/outbox"
	"order_service/internal/messaging/rabbitmq"
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
	if err := db.AutoMigrate(&entity.Order{}, &entity.OrderItem{}, &entity.OutboxEvent{}); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// ─── Step 4: Initialize Product Service gRPC Client ─────────────────────
	productClient, err := client.NewProductGRPCClient(cfg.ProductServiceGRPCAddr, cfg.ProductServiceTimeout)
	if err != nil {
		log.Fatalf("Failed to initialize Product Service gRPC client: %v", err)
	}

	// ─── Step 5: Initialize Layers (Repo -> Service -> Handler) ─────────────
	orderRepo := repository.NewOrderRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	orderService := service.NewOrderService(orderRepo, productClient)
	orderHandler := handler.NewOrderHandler(orderService)
	tokenHelper := helper.NewTokenHelper(cfg.JWTSecret)

	// ─── Step 6: Initialize RabbitMQ Client, Publisher & Outbox Dispatcher ───
	// The shared RabbitMQ connection and publisher are reused for the Dispatcher.
	// If RabbitMQ is temporarily unreachable at boot time, lazy reconnection is enabled
	// so order placement requests succeed immediately via PostgreSQL transactional outbox.
	rabbitClient, err := rabbitmq.NewClient(cfg.RabbitMQURL)
	if err != nil {
		log.Printf("Warning: Failed to connect to RabbitMQ at startup: %v. Initializing lazy reconnecting client.", err)
		rabbitClient = rabbitmq.NewClientLazy(cfg.RabbitMQURL)
	}

	rabbitPublisher, err := rabbitmq.NewPublisher(rabbitClient, rabbitmq.DefaultExchange)
	if err != nil {
		log.Printf("Warning: Failed to initialize RabbitMQ publisher: %v", err)
	}

	var outboxDispatcher *outbox.Dispatcher
	if rabbitPublisher != nil {
		outboxDispatcher = outbox.NewDispatcher(outboxRepo, rabbitPublisher, outbox.DefaultConfig())
	}

	// ─── Step 7: Initialize Combined Server Manager ─────────────────────────
	server := api.NewServer(
		cfg,
		db,
		orderHandler,
		tokenHelper,
		productClient,
		rabbitClient,
		rabbitPublisher,
		outboxDispatcher,
	)

	// ─── Step 8: Start Server with Graceful Shutdown ────────────────────────
	if err := server.Start(); err != nil {
		log.Fatalf("Order Service server error: %v", err)
	}
}
