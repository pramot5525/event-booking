package main

import (
	"booking-service/config"
	"booking-service/internal/client"
	bookinghttp "booking-service/internal/http"
	"booking-service/internal/model"
	"booking-service/internal/pkg/database"
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

	db, err := database.NewPostgres(cfg.DB.DSN())
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

	rdb, err := database.NewRedis(cfg.RDB.Host, cfg.RDB.Port, cfg.RDB.User, cfg.RDB.Password, cfg.RDB.Name)
	if err != nil {
		log.Fatal(err)
	}

	queueRepo := repository.NewQueueRepository(rdb)
	seatRepo := repository.NewSeatRepository(rdb)
	bookingRepo := repository.NewBookingRepository(db)
	eventClient := client.NewEventClient(cfg.EventServiceURL)

	bookingSvc := service.NewBookingService(bookingRepo, queueRepo, seatRepo, eventClient)

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
