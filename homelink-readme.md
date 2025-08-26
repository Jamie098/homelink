# HomeLink

> **Local Device Discovery and Communication Protocol**  
> A privacy-first, self-hosted alternative for home device communication

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

## Overview

HomeLink enables real-time communication between devices on your local network without requiring cloud services or external dependencies. Perfect for home labs, IoT devices, and privacy-conscious users who want to keep their data local.

**Key Features:**
- 🔒 **Privacy First** - All communication stays on your local network
- 🚀 **Real-time Events** - UDP multicast for instant device communication  
- 🔍 **Auto-Discovery** - Devices automatically find each other
- 📱 **Cross-Platform** - Works on any device that can run Go or connect via network
- 🏠 **Home Lab Ready** - Integrates with existing services like Frigate, *arr stack, etc.
- 🔧 **Simple Integration** - Easy to add to existing projects

## Quick Start

### Install
```bash
go get github.com/yourusername/homelink
```

### Basic Usage
```go
package main

import (
    "github.com/yourusername/homelink"
    "time"
)

func main() {
    // Create a HomeLink service
    service := homelink.NewHomeLinkService(
        "my-device-01",
        "My Home Server",
        []string{"file_sync", "notifications", "monitoring"},
    )
    
    // Start the service
    if err := service.Start(); err != nil {
        panic(err)
    }
    defer service.Stop()
    
    // Subscribe to events from other devices
    service.Subscribe([]string{"motion_detection", "door_sensor"})
    
    // Send an event
    service.SendEvent("file_uploaded", "New backup completed", map[string]string{
        "size": "1.2GB",
        "type": "daily_backup",
    })
    
    // Keep running
    select {}
}
```

## Use Cases

### Home Security Integration
```go
// Frigate sends motion alerts
service.SendEvent("motion_detection", "Motion detected at front door", map[string]string{
    "camera": "front_door",
    "confidence": "95",
})
```

### Media Server Notifications
```go
// Radarr/Sonarr completion notifications
service.SendEvent("download_complete", "Movie downloaded: The Matrix", map[string]string{
    "quality": "1080p",
    "size": "8.1GB",
})
```

### System Monitoring
```go
// TrueNAS health updates
service.SendEvent("storage_alert", "Disk usage above 90%", map[string]string{
    "pool": "main_pool",
    "usage": "92%",
})
```

## Architecture

HomeLink uses a simple but powerful architecture:

1. **Device Discovery**: Devices announce themselves via UDP multicast
2. **Event Subscription**: Devices subscribe to event types they care about
3. **Real-time Messaging**: Events are broadcast immediately to interested devices
4. **Heartbeat System**: Automatic detection of offline devices

### Message Types
- `device_announcement` - Device introduces itself to the network
- `subscribe` - Device requests specific event types
- `event` - Actual event data (motion detection, file uploads, etc.)
- `heartbeat` - Keep-alive messages

## Integration Examples

### Replace ntfy
Instead of:
```
Your Service → HTTP POST → ntfy → Phone polls → Notification
```

With HomeLink:
```
Your Service → UDP broadcast → Phone receives instantly → Notification
```

### Frigate Integration
```go
// Add to your Frigate webhook handler
service.SendEvent("person_detection", "Person detected", map[string]string{
    "camera": camera_name,
    "confidence": confidence_score,
    "snapshot": snapshot_url,
})
```

### Home Assistant Integration
```go
// Bridge HomeLink events to Home Assistant
go func() {
    for event := range service.EventChannel() {
        // Forward to Home Assistant MQTT or REST API
        publishToHomeAssistant(event)
    }
}()
```

## Mobile Apps

- **Flutter App** (coming soon) - Cross-platform mobile client
- **Android App** - Native Android client optimized for GrapheneOS
- **iOS App** - Native iOS client

## CLI Tool

Test and interact with HomeLink devices:

```bash
# Start the CLI tool
go run cmd/homelink-cli/main.go

# Commands:
> start "My Phone"
> subscribe motion_detection,door_sensor
> event test_alert "Testing HomeLink"
> devices
> quit
```

## Configuration

HomeLink works out of the box with sensible defaults, but you can customize:

```go
config := homelink.Config{
    MulticastAddr: "224.0.0.251:8080",  // Default multicast address
    HeartbeatInterval: 30 * time.Second, // How often to send heartbeats
    DeviceTimeout: 90 * time.Second,     // When to consider device offline
}

service := homelink.NewHomeLinkServiceWithConfig("device-id", "Device Name", capabilities, config)
```

## Roadmap

- [ ] **v1.0** - Core protocol implementation ✅
- [ ] **v1.1** - Flutter mobile app
- [ ] **v1.2** - Message persistence and reliability
- [ ] **v1.3** - Encryption for sensitive events
- [ ] **v2.0** - Remote access via relay servers
- [ ] **v2.1** - Web dashboard for device management
- [ ] **v2.2** - Plugin system for popular home services

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup
```bash
git clone https://github.com/yourusername/homelink
cd homelink
go mod tidy
go run examples/basic/main.go
```

### Running Tests
```bash
go test ./...
```

## Comparison with Alternatives

| Feature | HomeLink | ntfy | MQTT | Home Assistant |
|---------|----------|------|------|----------------|
| Local-only | ✅ | ❌ | ✅ | ✅ |
| Zero config | ✅ | ❌ | ❌ | ❌ |
| Auto-discovery | ✅ | ❌ | ❌ | ⚠️ |
| Real-time | ✅ | ⚠️ | ✅ | ✅ |
| Cross-platform | ✅ | ✅ | ✅ | ⚠️ |
| Mobile apps | 🚧 | ✅ | ⚠️ | ✅ |

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- 📖 [Documentation](https://homelink.dev/docs)
- 💬 [Discussions](https://github.com/yourusername/homelink/discussions)  
- 🐛 [Issues](https://github.com/yourusername/homelink/issues)
- 📧 Email: support@homelink.dev

---

**Built with ❤️ for the self-hosting community**