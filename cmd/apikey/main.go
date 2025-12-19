package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// Command-line tool to create a new API key in the database with customizable parameters.
// THIS is not a production-grade tool, just a simple utility for development/testing purposes.
func main() {
	// Define flags
	name := flag.String("name", "", "Name/description for the API key (required)")
	days := flag.Int("days", 0, "Number of days until expiration (0 = never expires)")
	pgURL := flag.String("postgres-url", os.Getenv("POSTGRES_URL"), "PostgreSQL connection URL")

	flag.Parse()

	if *name == "" {
		fmt.Println("Error: -name is required")
		flag.Usage()
		os.Exit(1)
	}

	if *pgURL == "" {
		fmt.Println("Error: PostgreSQL URL must be provided via -postgres-url flag or POSTGRES_URL env var")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// Connect to database
	store, err := postgres.NewPostgresStore(ctx, *pgURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()

	// Calculate expiration
	var expiresAt *time.Time
	if *days > 0 {
		expiry := time.Now().AddDate(0, 0, *days)
		expiresAt = &expiry
	}

	// Get default config values for API key format
	keyType := getEnv("MONO_API_KEY_TYPE", "sk")
	service := getEnv("MONO_API_SERVICE_NAME", "mono")
	version := getEnv("MONO_API_VERSION", "v1")

	// Generate API key with configurable prefix
	apiKey, err := auth.CreateAPIKey(ctx, store, keyType, service, version, *name, expiresAt)
	if err != nil {
		log.Fatalf("Failed to create API key: %v", err)
	}

	// Display result
	fmt.Println("\n✅ API Key created successfully!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Name: %s\n", *name)
	fmt.Printf("Format: %s-%s-%s-{short}-{long}\n", keyType, service, version)
	if expiresAt != nil {
		fmt.Printf("Expires: %s (%d days)\n", expiresAt.Format(time.RFC3339), *days)
	} else {
		fmt.Println("Expires: Never")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\nAPI Key: %s\n\n", apiKey)
	fmt.Println("⚠️  IMPORTANT: Save this key now! It will not be shown again.")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Usage example:")
	fmt.Printf("  curl -H \"Authorization: Bearer %s\" http://localhost:8081/v1/lists\n", apiKey)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
