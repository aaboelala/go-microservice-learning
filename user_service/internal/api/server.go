package api

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ahmed/go-service/config"
	"github.com/ahmed/go-service/internal/api/rest"
	"github.com/ahmed/go-service/internal/api/rest/handler"
	"github.com/ahmed/go-service/internal/helper"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

// Server encapsulates the Fiber web application.
type Server struct {
	app *fiber.App
	cfg *config.Config
	db  *gorm.DB
}

// NewServer initializes Fiber server with middleware and health checks.
func NewServer(cfg *config.Config, db *gorm.DB, userHandler *handler.UserHandler, h *helper.Helper) *Server {
	app := fiber.New(fiber.Config{
		AppName: "E-Commerce User Service",
	})

	// Global Logger & Recover Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Health check endpoints
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "user-service"})
	})

	app.Get("/ready", func(c *fiber.Ctx) error {
		sqlDB, err := db.DB()
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "not ready", "error": "db error"})
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "not ready", "error": "db ping failed"})
		}
		return c.JSON(fiber.Map{"status": "ready", "database": "connected"})
	})

	// Setup REST API routes
	rest.SetupRoutes(app, userHandler, h)

	return &Server{
		app: app,
		cfg: cfg,
		db:  db,
	}
}

// Start listens for HTTP traffic and performs graceful shutdown on termination signals.
func (s *Server) Start() {
	go func() {
		addr := fmt.Sprintf(":%s", s.cfg.ServerPort)
		fmt.Printf("🚀 Server running on %s\n", addr)
		if err := s.app.Listen(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n⏳ Gracefully shutting down server...")
	if err := s.app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	sqlDB, err := s.db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}

	fmt.Println("✅ Server stopped")
}
