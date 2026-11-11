package api

import (
	"context"
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

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Server coordinates the HTTP server lifecycle and its underlying dependencies.
type Server struct {
	app           *fiber.App
	db            *gorm.DB
	cfg           *config.Config
	productClient client.ProductClient
}

// NewServer initializes the Fiber app, registers health and order routes, and returns a Server.
func NewServer(
	cfg *config.Config,
	db *gorm.DB,
	orderHandler *handler.OrderHandler,
	tokenHelper *helper.TokenHelper,
	productClient client.ProductClient,
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
		app:           app,
		db:            db,
		cfg:           cfg,
		productClient: productClient,
	}
}

// Start runs the HTTP server and blocks until an OS interrupt or termination signal is received.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.ServerPort)

	errChan := make(chan error, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

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

	// 1. Gracefully shutdown Fiber HTTP server
	log.Println("Shutting down HTTP server...")
	if err := s.app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// 2. Close Product Service gRPC Client Connection
	if s.productClient != nil {
		log.Println("Closing Product Service gRPC connection...")
		if err := s.productClient.Close(); err != nil {
			log.Printf("Error closing gRPC client connection: %v", err)
		}
	}

	// 3. Close Database Connection Pool
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

