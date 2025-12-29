package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// Default values for API key format when not configured.
const (
	defaultKeyType     = "sk"
	defaultServiceName = "mono"
	defaultVersion     = "v1"
)

// Command-line tool to create a new API key in the database with customizable parameters.
// THIS is not a production-grade tool, just a simple utility for development/testing purposes.
func main() {
	// Define flags
	name := flag.String("name", "", "Name/description for the API key (required)")
	days := flag.Int("days", 0, "Number of days until expiration (0 = never expires)")

	flag.Parse()

	// Load configuration
	cfg, err := config.LoadAPIKeyGenConfig(*name, *days)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		flag.Usage()
		log.Fatal(err)
	}

	ctx := context.Background()

	// Connect to database
	store, err := postgres.NewStoreWithConfig(ctx, postgres.DBConfig{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.Database.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime: time.Duration(cfg.Database.ConnMaxIdleTime) * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("Failed to close store: %v", err)
		}
	}()

	// Calculate expiration
	var expiresAt *time.Time
	if cfg.DaysValid > 0 {
		expiry := time.Now().UTC().AddDate(0, 0, cfg.DaysValid)
		expiresAt = &expiry
	}

	// Apply defaults for API key format
	keyType := cfg.APIKey.KeyType
	if keyType == "" {
		keyType = defaultKeyType
	}
	serviceName := cfg.APIKey.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	version := cfg.APIKey.Version
	if version == "" {
		version = defaultVersion
	}

	// Generate API key with configurable prefix
	apiKey, err := auth.CreateAPIKey(ctx, store, keyType, serviceName, version, cfg.Name, expiresAt)
	if err != nil {
		log.Fatalf("Failed to create API key: %v", err)
	}

	// Display result
	fmt.Println("\n API Key created successfully!")
	fmt.Println("----------------------------------------")
	fmt.Printf("Name: %s\n", cfg.Name)
	fmt.Printf("Format: %s-%s-%s-{short}-{long}\n", keyType, serviceName, version)
	if expiresAt != nil {
		fmt.Printf("Expires: %s (%d days)\n", expiresAt.Format(time.RFC3339), cfg.DaysValid)
	} else {
		fmt.Println("Expires: Never")
	}
	fmt.Println("----------------------------------------")
	fmt.Printf("\nAPI Key: %s\n\n", apiKey)
	fmt.Println("IMPORTANT: Save this key now! It will not be shown again.")
	fmt.Println("----------------------------------------")
	fmt.Println("Usage example:")
	fmt.Printf("  curl -H \"Authorization: Bearer %s\" http://localhost:8081/v1/lists\n", apiKey)
}
