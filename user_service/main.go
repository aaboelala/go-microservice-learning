package main

import (
	"log"

	"github.com/ahmed/go-service/config"
	"github.com/ahmed/go-service/internal/api"
	"github.com/ahmed/go-service/internal/api/rest/handler"
	"github.com/ahmed/go-service/internal/domain"
	"github.com/ahmed/go-service/internal/helper"
	"github.com/ahmed/go-service/internal/repository"
	"github.com/ahmed/go-service/internal/service"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// 1. Load Environment Settings
	cfg := config.LoadConfig()

	// 2. Connect to Database & Run GORM AutoMigrations
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&domain.User{}); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// 3. Construct Security Helper (bcrypt & JWT)
	h := helper.NewHelper(cfg.JWTSecret, cfg.JWTExpiration)

	// 4. Construct Repository Layer
	userRepo := repository.NewUserRepository(db)

	// 5. Construct Service Layer (Inject repo & helper)
	userSvc := service.NewUserService(userRepo, h)

	// 6. Construct Handler Layer (Inject service)
	userHandler := handler.NewUserHandler(userSvc)

	// 7. Construct & Start API Server
	srv := api.NewServer(cfg, db, userHandler, h)
	srv.Start()
}
