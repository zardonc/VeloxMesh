package main

import (
	"log"
	"veloxmesh/internal/app"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // Ignore error, it will just use os env if file doesn't exist

	application := app.New()
	if err := application.Run(); err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
}
