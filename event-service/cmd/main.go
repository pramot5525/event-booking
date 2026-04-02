package main

import (
	"event-service/config"
	httpAdapter "event-service/internal/http"
	"event-service/internal/model"
	"event-service/internal/pkg/database"
	"event-service/internal/repository"
	"event-service/internal/service"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := database.NewPostgres(cfg)
	if err != nil {
		log.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	rdb, err := database.NewRedis(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rdb.Close()

	if err := db.AutoMigrate(&model.Event{}); err != nil {
		log.Fatal(err)
	}

	eventRepo := repository.NewEventRepository(db)
	eventService := service.NewEventService(eventRepo, rdb, cfg.CacheTTL)

	app := fiber.New()
	// Setup routes
	httpAdapter.NewRouter(app, eventService)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("shutting down...")
		_ = app.Shutdown()
	}()

	log.Fatal(app.Listen(":" + cfg.AppPort))
}
