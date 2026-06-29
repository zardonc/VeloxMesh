package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"veloxmesh/internal/storage"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	addr := os.Getenv("QDRANT_ADDR")
	apiKey := os.Getenv("QDRANT_API_KEY")

	if addr == "" || apiKey == "" {
		log.Fatal("Missing QDRANT_ADDR or QDRANT_API_KEY")
	}

	fmt.Printf("Connecting to Qdrant at %s...\n", addr)
	adapter, err := storage.NewQdrantVectorAdapter(addr, apiKey)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	fmt.Println("Connected successfully!")
	
	// Try inserting a vector
	vectors := [][]float32{{0.1, 0.2, 0.3}}
	metadata := []map[string]interface{}{{"test_key": "test_value"}}
	err = adapter.Insert(context.Background(), "test_collection", vectors, metadata)
	if err != nil {
		log.Printf("Insert failed (could be expected if collection doesn't exist): %v", err)
	} else {
		fmt.Println("Inserted vector successfully!")
	}
}
