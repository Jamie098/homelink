# HomeLink

> **Local Device Discovery and Communication Protocol**  
> A privacy-first, self-hosted alternative for home device communication

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## Overview

HomeLink enables real-time communication between devices on your local network without requiring cloud services or external dependencies. Perfect for home automation, security systems, and any application that needs to publish events to other devices on the network.

**Key Features:**
- 🔒 **Privacy First** - All communication stays on your local network
- 🚀 **Real-time Events** - UDP multicast for instant device communication  
- 🔍 **Auto-Discovery** - Devices automatically find each other
- 📱 **Cross-Platform** - Works on any device that can run Go
- � **HTTP API** - Simple REST API for event publishing
- 🔧 **Docker Ready** - Easy deployment with Docker

## Quick Start

### Docker (Recommended)

```bash
# Build and run
docker build -t homelink .
docker run -d \
  -e HOMELINK_DEVICE_ID=my-service \
  -e HOMELINK_DEVICE_NAME="My HomeLink Service" \
  -e HOMELINK_CAPABILITIES=event_publisher,api \
  -p 8080:8080 -p 8081:8081 \
  homelink
```

### Go Install
```bash
go mod tidy
go run cmd/homelink-service/main.go
```
## API Usage

Once running, the service exposes an HTTP API on port 8081:

### Publish Events
```bash
curl -X POST http://localhost:8081/publish \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "motion_detected",
    "description": "Motion detected at front door",
    "data": {
      "camera": "front_door",
      "confidence": "95"
    }
  }'
```

### Subscribe to Events
```bash
curl -X POST http://localhost:8081/subscribe \
  -H "Content-Type: application/json" \
  -d '{
    "event_types": ["motion_detected", "person_detected"]
  }'
```

### List Devices
```bash
curl http://localhost:8081/devices
```

### Health Check
```bash
curl http://localhost:8081/health
```

## Configuration

Configure the service using environment variables:

- `HOMELINK_DEVICE_ID` - Unique identifier for this device
- `HOMELINK_DEVICE_NAME` - Human-readable name  
- `HOMELINK_CAPABILITIES` - Comma-separated capabilities
- `HOMELINK_API_PORT` - HTTP API port (default: 8081)

## Integration

HomeLink is designed to be easily integrated into any system that needs to publish events. Common use cases:

## Architecture

HomeLink uses a simple but powerful architecture:

1. **Device Discovery**: Devices announce themselves via UDP multicast on port 8080
2. **Event Publishing**: Events are broadcast to all devices on the network
3. **Event Subscription**: Devices can subscribe to specific event types
4. **HTTP API**: Simple REST API for external integration

### Message Types
- `device_announcement` - Device introduces itself to the network
- `subscribe` - Device requests specific event types  
- `event` - Actual event data (motion detection, file uploads, etc.)
- `heartbeat` - Keep-alive messages

## License

MIT License - see [LICENSE](LICENSE) for details.

- 📖 [Documentation](https://homelink.dev/docs)
- 💬 [Discussions](https://github.com/yourusername/homelink/discussions)  
- 🐛 [Issues](https://github.com/yourusername/homelink/issues)
- 📧 Email: support@homelink.dev

---

**Built with ❤️ for the self-hosting community**