package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"order_service/config"
	"order_service/internal/api/rest"
	"order_service/internal/api/rest/handler"
	"order_service/internal/client"
	"order_service/internal/helper"
	"order_service/internal/messaging/outbox"
	"order_service/internal/messaging/rabbitmq"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Server coordinates the HTTP server lifecycle and its underlying dependencies.
type Server struct {
	app              *fiber.App
	db               *gorm.DB
	cfg              *config.Config
	productClient    client.ProductClient
	rabbitClient     *rabbitmq.Client
	rabbitPublisher  rabbitmq.EventPublisher
	outboxDispatcher *outbox.Dispatcher
}

// NewServer initializes the Fiber app, registers health and order routes, and returns a Server.
func NewServer(
	cfg *config.Config,
	db *gorm.DB,
	orderHandler *handler.OrderHandler,
	tokenHelper *helper.TokenHelper,
	productClient client.ProductClient,
	rabbitClient *rabbitmq.Client,
	rabbitPublisher rabbitmq.EventPublisher,
	outboxDispatcher *outbox.Dispatcher,
) *Server {
	app := fiber.New(fiber.Config{
		CaseSensitive: true,
		StrictRouting: true,
		BodyLimit:     10 * 1024 * 1024,
		AppName:       "OrderService",
	})

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		if db != nil {
			sqlDB, err := db.DB()
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"status": "error",
					"error":  "Failed to get database handle",
				})
			}
			ctxWithTimeout, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()

			if err := sqlDB.PingContext(ctxWithTimeout); err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"status": "error",
					"error":  "Database connection is unhealthy",
				})
			}
		}

		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	rest.SetupRoutes(app, orderHandler, tokenHelper)

	return &Server{
		app:              app,
		db:               db,
		cfg:              cfg,
		productClient:    productClient,
		rabbitClient:     rabbitClient,
		rabbitPublisher:  rabbitPublisher,
		outboxDispatcher: outboxDispatcher,
	}
}

// Start runs the HTTP server and blocks until an OS interrupt or termination signal is received.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.ServerPort)

	errChan := make(chan error, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Managed context for background worker goroutines
	dispatcherCtx, cancelDispatcher := context.WithCancel(context.Background())
	defer cancelDispatcher()

	// Start Outbox Dispatcher in a managed background goroutine
	if s.outboxDispatcher != nil {
		go func() {
			log.Println("Starting Outbox Dispatcher background worker...")
			if err := s.outboxDispatcher.Start(dispatcherCtx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Outbox Dispatcher stopped with error: %v", err)
			}
		}()
	}

	go func() {
		log.Printf("Starting Order Service HTTP server on %s", addr)
		if err := s.app.Listen(addr); err != nil {
			errChan <- fmt.Errorf("HTTP server listen error: %w", err)
		}
	}()

	var serverErr error
	select {
	case sig := <-quit:
		log.Printf("Received termination signal %v. Initiating graceful shutdown...", sig)
	case err := <-errChan:
		log.Printf("Server failure detected: %v. Initiating shutdown...", err)
		serverErr = err
	}

	// 1. Gracefully stop Outbox Dispatcher so it drains/completes active batch without leaking goroutines
	if s.outboxDispatcher != nil {
		log.Println("Stopping Outbox Dispatcher...")
		s.outboxDispatcher.Stop()
		cancelDispatcher()
	}

	// 2. Gracefully shutdown Fiber HTTP server
	log.Println("Shutting down HTTP server...")
	if err := s.app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// 3. Close RabbitMQ Publisher and Connection
	if s.rabbitPublisher != nil {
		log.Println("Closing RabbitMQ Publisher...")
		if err := s.rabbitPublisher.Close(); err != nil {
			log.Printf("Error closing RabbitMQ publisher: %v", err)
		}
	}
	if s.rabbitClient != nil {
		log.Println("Closing RabbitMQ connection...")
		if err := s.rabbitClient.Close(); err != nil {
			log.Printf("Error closing RabbitMQ connection: %v", err)
		}
	}

	// 4. Close Product Service gRPC Client Connection
	if s.productClient != nil {
		log.Println("Closing Product Service gRPC connection...")
		if err := s.productClient.Close(); err != nil {
			log.Printf("Error closing gRPC client connection: %v", err)
		}
	}

	// 5. Close Database Connection Pool
	if s.db != nil {
		log.Println("Closing database connections...")
		sqlDB, err := s.db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	log.Println("Order Service stopped gracefully")
	return serverErr
}

// App returns the underlying Fiber app (useful for integration testing).
func (s *Server) App() *fiber.App {
	return s.app
}
