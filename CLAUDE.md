# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

### Main Service
```bash
# Build the main HomeLink service
go build -o homelink cmd/homelink-service/main.go

# Run the service directly
go run cmd/homelink-service/main.go

# Build with Docker
docker build -t homelink .
```

### Frigate Bridge
```bash
# Build the Frigate bridge service
cd frigate-bridge
go build -o frigate-bridge .

# Run tests
go test ./...
```

### Development Commands
```bash
# Tidy dependencies
go mod tidy

# Format code
gofmt -w .

# Run all tests (if any exist)
go test ./...
```

## Architecture Overview

HomeLink is a privacy-first local network device discovery and communication protocol built in Go. The architecture consists of:

### Core Components

- **Service Layer** (`service.go`): Main service implementation with dependency injection pattern
- **Network Layer** (`network.go`, `network_helpers.go`): UDP multicast communication and helper utilities
- **Device Management** (`devices.go`): Device discovery, tracking, and management
- **Message System** (`message_factory.go`, `types.go`): Centralized message creation and data structures
- **Event System** (`events.go`): Event publishing, subscription, and handling

### Advanced Features

- **Security** (`security.go`): Optional encryption, device authentication, QR code pairing
- **Storage** (`storage.go`): SQLite-based event persistence with configurable retention
- **Health Monitoring** (`health.go`): System health tracking and metrics
- **Reliability** (`reliability.go`): Message delivery guarantees and retry mechanisms
- **Filtering** (`filtering.go`): Event filtering and processing rules
- **Notifications** (`notifications.go`): External notification integrations
- **Dashboard** (`dashboard.go`): Web-based monitoring interface

### Protocol Support

- **JSON Protocol**: Default human-readable format
- **Binary Protocol** (`binary_protocol.go`): Efficient binary encoding for high-throughput scenarios
- **Auto Protocol**: Automatic protocol selection based on device capabilities

## Key Patterns

### Service Construction
The service uses a builder pattern with multiple constructors:
- `NewHomeLinkService()`: Basic service
- `NewSecureHomeLinkService()`: With security features
- `NewAdvancedHomeLinkService()`: With all advanced features

### Error Handling
Centralized error handling through `ErrorHandler` (`errors.go`) with device-specific context and consistent logging patterns.

### Message Factory
All messages are created through `MessageFactory` (`message_factory.go`) to ensure consistency and reduce duplication.

### Configuration Validation
`ConfigValidator` (`config_validator.go`) provides comprehensive validation for all configuration parameters with input sanitization.

## Environment Configuration

The service is configured via environment variables (see `cmd/homelink-service/main.go`):

### Core Configuration
- `HOMELINK_DEVICE_ID`: Unique device identifier
- `HOMELINK_DEVICE_NAME`: Human-readable device name
- `HOMELINK_CAPABILITIES`: Comma-separated capabilities list
- `HOMELINK_API_PORT`: HTTP API port (default: 8081)

### Security Configuration
- `HOMELINK_SECURITY_ENABLED`: Enable security features
- `HOMELINK_REQUIRE_AUTH`: Require device authentication
- `HOMELINK_NETWORK_KEY`: Pre-shared network key
- `HOMELINK_RATE_LIMIT`: Rate limiting (messages/second)

### Storage Configuration
- `HOMELINK_STORAGE_ENABLED`: Enable event storage
- `HOMELINK_DATABASE_PATH`: SQLite database path
- `HOMELINK_RETENTION_DAYS`: Event retention period
- `HOMELINK_BACKUP_ENABLED`: Enable automatic backups

## Network Protocol

- **Discovery Port**: 8080 (UDP multicast)
- **API Port**: 8081 (HTTP REST API, configurable)
- **Multicast Group**: Uses standard multicast for device discovery
- **Message Types**: device_announcement, subscribe, event, heartbeat

## Frigate Bridge

The `frigate-bridge/` directory contains a standalone service that bridges Frigate NVR detection events to HomeLink:

- **Purpose**: Real-time integration between Frigate security system and HomeLink network
- **Architecture**: Polls Frigate API, transforms events, publishes to HomeLink
- **Configuration**: Environment variables for URLs, filtering, and reliability settings
- **Features**: Event filtering by camera/confidence, reliable delivery, health monitoring

## File Organization

### Core Files
- `homelink.go`: Legacy entry point with usage examples
- `service.go`: Main service implementation
- `constants.go`: Protocol constants and message types
- `types.go`: Core data structures and interfaces

### Feature Modules
- Security: `security.go`, `ratelimiter.go`
- Storage: `storage.go` 
- Monitoring: `health.go`, `dashboard.go`
- Advanced: `filtering.go`, `notifications.go`, `reliability.go`

### Utilities
- `message_factory.go`: Centralized message creation
- `errors.go`: Unified error handling
- `config_validator.go`: Configuration validation
- `network_helpers.go`: Network utility functions

### Documentation
- `homelink-readme.md`: Main project documentation
- `REFACTORING_SUMMARY.md`: Details on recent code improvements

## Development Notes

- The codebase was recently refactored for better maintainability (see `REFACTORING_SUMMARY.md`)
- Uses dependency injection patterns for utilities and components  
- All public APIs maintain backwards compatibility
- New features should follow established patterns for error handling and message creation
- Security features are optional but recommended for production deployments