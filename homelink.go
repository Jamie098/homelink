// HomeLink - Legacy Main File (Use service.go for the core service)
// Local Device Discovery and Communication Protocol
// Copyright (c) 2025 - Open Source Project
// A privacy-first, self-hosted alternative for home device communication
//
// This file is being refactored. Core functionality has been moved to:
// - constants.go: Protocol constants and message types
// - types.go: Core data structures
// - events.go: Event system
// - network.go: Network communication
// - devices.go: Device discovery and management
// - service.go: Main service implementation

package homelink

// This file now serves as the main package entry point and contains
// example usage. The actual implementation is in the other files.

/*
Example usage - This would typically be in a separate main.go file:

func main() {
	// Create a HomeLink service instance
	service := NewHomeLinkService(
		"security-01",
		"Security System",
		[]string{"motion_detection", "person_detection", "vehicle_detection"},
	)

	// Start the service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	// Subscribe to events we care about
	service.Subscribe([]string{"motion_detection", "person_detection"})

	// Simulate sending events (this would normally be triggered by external systems)
	go func() {
		time.Sleep(5 * time.Second)
		service.SendEvent("person_detection", "Person detected at front door", map[string]string{
			"camera":    "front_door",
			"confidence": "95",
			"timestamp":  fmt.Sprintf("%d", time.Now().Unix()),
		})
	}()

	// Keep running
	select {}
}
*/
