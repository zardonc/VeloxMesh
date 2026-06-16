package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"veloxmesh/internal/app"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // Ignore error, it will just use os env if file doesn't exist

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	application, err := app.New()
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}
	if err := application.Run(ctx); err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
}
