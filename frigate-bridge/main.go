package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize configuration
	config := NewConfig()
	if err := config.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	log.Printf("Starting Frigate-HomeLink Bridge Service")
	log.Printf("  Frigate URL: %s", config.FrigateURL)
	log.Printf("  HomeLink URL: %s", config.HomeLinkURL)
	log.Printf("  Poll Interval: %s", config.PollInterval)
	log.Printf("  Bridge ID: %s", config.BridgeID)

	// Create Frigate API client
	frigateClient := NewFrigateClient(config.FrigateURL, config.FrigateTimeout)

	// Create HomeLink client
	homelinkClient := NewHomeLinkClient(config.HomeLinkURL, config.HomeLinkAPIKey, config.HomeLinkTimeout)

	// Create event transformer
	transformer := NewEventTransformer(config.BridgeID, frigateClient)

	// Create bridge service
	bridge := NewBridgeService(frigateClient, homelinkClient, transformer, config)

	// Start the bridge service
	if err := bridge.Start(); err != nil {
		log.Fatalf("Failed to start bridge service: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Frigate-HomeLink bridge is running. Press Ctrl+C to stop.")
	<-sigChan

	log.Println("Shutting down...")
	bridge.Stop()
	log.Println("Shutdown complete")
}