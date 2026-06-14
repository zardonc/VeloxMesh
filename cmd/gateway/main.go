package main

import (
	"log"
	"veloxmesh/internal/app"
)

func main() {
	application := app.New()
	if err := application.Run(); err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
}
