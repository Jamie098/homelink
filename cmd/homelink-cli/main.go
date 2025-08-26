package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"homelink" // Import our HomeLink package
)

// This is a simple CLI tool to interact with the HomeLink Protocol
// It demonstrates how to integrate the protocol into your existing services

func main() {
	fmt.Println("HomeLink Protocol CLI Tool")
	fmt.Println("==========================")
	fmt.Println("Commands:")
	fmt.Println("  start <device_name> - Start a HomeLink device")
	fmt.Println("  event <type> <description> - Send an event")
	fmt.Println("  devices - List discovered devices")
	fmt.Println("  subscribe <event_types> - Subscribe to events (comma separated)")
	fmt.Println("  discover - Trigger device discovery")
	fmt.Println("  stats - Show discovery statistics")
	fmt.Println("  quit - Exit")
	fmt.Println()

	var service *homelink.HomeLinkService
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		command := parts[0]

		switch command {
		case "start":
			if len(parts) < 2 {
				fmt.Println("Usage: start <device_name>")
				continue
			}

			deviceName := strings.Join(parts[1:], " ")
			deviceID := fmt.Sprintf("cli-%d", time.Now().Unix())

			// Create HomeLink service with some example capabilities
			service = homelink.NewHomeLinkService(
				deviceID,
				deviceName,
				[]string{"manual_control", "testing", "cli_interface"},
			)

			if err := service.Start(); err != nil {
				fmt.Printf("Failed to start service: %v\n", err)
				continue
			}

			fmt.Printf("Started device: %s (ID: %s)\n", deviceName, deviceID)

		case "event":
			if service == nil {
				fmt.Println("Please start a device first using 'start <device_name>'")
				continue
			}
			if len(parts) < 3 {
				fmt.Println("Usage: event <type> <description>")
				continue
			}

			eventType := parts[1]
			description := strings.Join(parts[2:], " ")

			service.SendEvent(eventType, description, map[string]string{
				"source": "cli",
				"time":   time.Now().Format(time.RFC3339),
			})

			fmt.Printf("Sent event: %s - %s\n", eventType, description)

		case "devices":
			if service == nil {
				fmt.Println("Please start a device first using 'start <device_name>'")
				continue
			}

			devices := service.GetDevices()
			if len(devices) == 0 {
				fmt.Println("No devices discovered yet")
			} else {
				fmt.Println("Discovered devices:")
				for _, device := range devices {
					fmt.Printf("  - %s (%s)\n", device.Name, device.ID)
					fmt.Printf("    Capabilities: %v\n", device.Capabilities)
					fmt.Printf("    Last seen: %s\n", device.LastSeen.Format(time.RFC3339))
				}
			}

		case "subscribe":
			if service == nil {
				fmt.Println("Please start a device first using 'start <device_name>'")
				continue
			}
			if len(parts) < 2 {
				fmt.Println("Usage: subscribe <event_types> (comma separated)")
				fmt.Println("Example: subscribe motion_detection,person_detection")
				continue
			}

			eventTypesStr := strings.Join(parts[1:], " ")
			eventTypes := strings.Split(eventTypesStr, ",")

			// Trim whitespace
			for i, et := range eventTypes {
				eventTypes[i] = strings.TrimSpace(et)
			}

			service.Subscribe(eventTypes)
			fmt.Printf("Subscribed to: %v\n", eventTypes)

		case "discover":
			if service == nil {
				fmt.Println("Please start a device first using 'start <device_name>'")
				continue
			}

			service.TriggerDiscovery()
			fmt.Println("Device discovery triggered")

		case "stats":
			if service == nil {
				fmt.Println("Please start a device first using 'start <device_name>'")
				continue
			}

			stats := service.GetDiscoveryStats()
			fmt.Printf("Discovery Statistics:\n")
			fmt.Printf("  Total devices: %v\n", stats["total_devices"])
			fmt.Printf("  Subscriptions: %v\n", stats["subscription_count"])
			fmt.Printf("  Devices:\n")

			devices := stats["devices"].([]map[string]interface{})
			if len(devices) == 0 {
				fmt.Printf("    (none discovered)\n")
			} else {
				for _, device := range devices {
					fmt.Printf("    - %s (%s) at %s\n",
						device["name"], device["id"], device["address"])
					fmt.Printf("      Capabilities: %v\n", device["capabilities"])
					fmt.Printf("      Last seen: %s\n", device["last_seen"])
				}
			}

		case "quit", "exit", "q":
			if service != nil {
				service.Stop()
			}
			fmt.Println("Goodbye!")
			return

		default:
			fmt.Printf("Unknown command: %s\n", command)
		}
	}
}

// Example scenarios to try:
// 1. Run this CLI in two terminals
// 2. In terminal 1: start "My Phone"
// 3. In terminal 2: start "Frigate Server"
// 4. In both terminals: devices (should see each other)
// 5. In terminal 1: subscribe motion_detection,person_detection
// 6. In terminal 2: event motion_detection "Motion detected at front door"
// 7. Check terminal 1 for the received event
