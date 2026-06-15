package main

import (
	"github.com/joho/godotenv"
	"log"
	"veloxmesh/internal/app"
)

func main() {
	_ = godotenv.Load() // Ignore error, it will just use os env if file doesn't exist

	application, err := app.New()
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}
	if err := application.Run(); err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
}
