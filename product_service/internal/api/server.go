package api

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"product_service/config"
	"product_service/internal/api/rest"
	"product_service/internal/api/rest/handler"
	"product_service/internal/help"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type Server struct {
	app        *fiber.App
	grpcServer *grpc.Server
	db         *gorm.DB
	cfg        *config.Config
}

// NewServer initializes the combined HTTP REST server and gRPC server.
func NewServer(
	cfg *config.Config,
	db *gorm.DB,
	userHandler *handler.UserHandler,
	helpHandler *help.HelpMethod,
	grpcServer *grpc.Server,
) *Server {
	app := fiber.New(fiber.Config{
		// Enable case-sensitive routing
		CaseSensitive: true,
		// Enable strict routing
		StrictRouting: true,
		// Enable request body size limit (e.g., 10MB)
		BodyLimit: 10 * 1024 * 1024, // 10MB
		AppName:   "ProductService",
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		sqlDB, err := db.DB()
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"status": "error",
				"error":  "Failed to get database connection",
			})
		}
		ctxWithTimeout, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()

		if err := sqlDB.PingContext(ctxWithTimeout); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"status": "error",
				"error":  "Database connection is not healthy",
			})
		}

		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	rest.HandleRequests(app, userHandler, helpHandler)

	return &Server{
		app:        app,
		grpcServer: grpcServer,
		db:         db,
		cfg:        cfg,
	}
}

// Start launches both REST and gRPC servers concurrently and manages graceful shutdown.
func (s *Server) Start() error {
	grpcAddr := fmt.Sprintf(":%s", s.cfg.GRPCPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC address %s: %w", grpcAddr, err)
	}

	errChan := make(chan error, 2)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 1. Start gRPC server
	go func() {
		log.Printf("Starting gRPC server on %s", grpcAddr)
		if err := s.grpcServer.Serve(grpcLis); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// 2. Start REST server
	restAddr := fmt.Sprintf(":%s", s.cfg.ServerPort)
	go func() {
		log.Printf("Starting REST server on %s", restAddr)
		if err := s.app.Listen(restAddr); err != nil {
			errChan <- fmt.Errorf("REST server error: %w", err)
		}
	}()

	// Wait for termination signal or server error
	var serverErr error
	select {
	case sig := <-quit:
		log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)
	case err := <-errChan:
		log.Printf("Server failure detected: %v. Initiating shutdown...", err)
		serverErr = err
	}

	// 3. Graceful shutdown gRPC server
	log.Println("Shutting down gRPC server...")
	grpcStopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(grpcStopped)
	}()

	select {
	case <-grpcStopped:
		log.Println("gRPC server stopped cleanly")
	case <-time.After(5 * time.Second):
		log.Println("gRPC server graceful shutdown timed out, force stopping...")
		s.grpcServer.Stop()
	}

	// 4. Graceful shutdown REST server
	log.Println("Shutting down REST server...")
	if err := s.app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("REST server shutdown error: %v", err)
	}

	// 5. Close Database Connection
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	fmt.Println("server is stopped")
	return serverErr
}
