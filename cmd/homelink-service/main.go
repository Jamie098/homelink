package main

import (
	"homelink"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Create a HomeLink service instance
	service := homelink.NewHomeLinkService(
		"docker-service",
		"HomeLink Docker Service",
		[]string{"docker", "service", "automation"},
	)

	// Start the service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start HomeLink service: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("HomeLink service running. Press Ctrl+C to stop.")
	<-sigChan

	log.Println("Shutting down...")
	service.Stop()
}
