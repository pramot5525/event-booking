package main

import (
	"booking-service/config"
	httpAdapter "booking-service/internal/http"
	"booking-service/internal/model"
	"booking-service/internal/pkg/database"
	"booking-service/internal/repository"
	"booking-service/internal/service"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	if err := db.AutoMigrate(&model.Booking{}); err != nil {
		log.Fatal(err)
	}

	bookingRepo := repository.NewBookingRepository(db)
	eventClient := service.NewEventClient(cfg.EventServiceURL)
	bookingService := service.NewBookingService(bookingRepo, rdb, eventClient, cfg)

	app := fiber.New()
	httpAdapter.NewRouter(app, bookingService)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = app.ShutdownWithContext(ctx)
	}()

	log.Fatal(app.Listen(":" + cfg.AppPort))
}
