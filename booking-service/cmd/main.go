package main

import (
	"booking-service/config"
	bookinghttp "booking-service/internal/http"
	"booking-service/internal/client"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"booking-service/internal/service"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
)

func main() {
	cfg := config.Load()

	db, err := config.NewPostgres(cfg)
	if err != nil {
		log.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	if err := db.AutoMigrate(&model.Booking{}); err != nil {
		log.Fatal(err)
	}

	rdb, err := config.NewRedis(cfg)
	if err != nil {
		log.Fatal(err)
	}

	lockRepo    := repository.NewLockRepository(rdb)
	queueRepo   := repository.NewQueueRepository(rdb)
	seatRepo    := repository.NewSeatRepository(rdb)
	bookingRepo := repository.NewBookingRepository(db)
	eventClient := client.NewEventClient(cfg.EventServiceURL)

	bookingSvc := service.NewBookingService(bookingRepo, lockRepo, queueRepo, seatRepo, eventClient)

	app := fiber.New()
	bookinghttp.NewRouter(app, bookingSvc)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("shutting down...")
		_ = app.Shutdown()
	}()

	log.Fatal(app.Listen(":" + cfg.AppPort))
}
