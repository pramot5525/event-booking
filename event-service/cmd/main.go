package main

import (
	"event-service/config"
	httpAdapter "event-service/internal/http"
	"event-service/internal/model"
	"event-service/internal/repository"
	"event-service/internal/service"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := gorm.Open(postgres.Open(cfg.DB.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	if err := db.AutoMigrate(&model.Event{}); err != nil {
		log.Fatal(err)
	}

	eventRepo := repository.NewEventRepository(db)
	eventService := service.NewEventService(eventRepo)

	app := fiber.New()
	httpAdapter.NewRouter(app, eventService)

	log.Fatal(app.Listen(":" + cfg.AppPort))
}
