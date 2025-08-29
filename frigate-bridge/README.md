# Frigate-HomeLink Bridge

A standalone bridge service that connects Frigate NVR to HomeLink, enabling Frigate detection events to be shared across your HomeLink network.

## Features

- **Real-time Event Bridging**: Polls Frigate API and forwards events to HomeLink in real-time
- **Configurable Filtering**: Filter events by camera, object type, and confidence level
- **Reliable Delivery**: Supports HomeLink's reliable event delivery for high-priority events
- **Health Monitoring**: Built-in health checks and statistics endpoint
- **Docker Support**: Easy deployment with Docker and Docker Compose
- **Retry Logic**: Automatic retry for failed event publishing
- **Memory Management**: Automatic cleanup of processed events to prevent memory growth

## Architecture

```
┌─────────────┐    HTTP API    ┌─────────────────┐    HTTP API    ┌─────────────┐
│   Frigate   │◄──────────────►│ Frigate-Bridge  │◄──────────────►│  HomeLink   │
│     NVR     │    (polling)   │     Service     │   (events)     │   Service   │
└─────────────┘                └─────────────────┘                └─────────────┘
```

The bridge service:
1. Polls Frigate's `/api/events` endpoint for new detection events
2. Filters events based on your configuration (cameras, object types, confidence)
3. Transforms Frigate events into HomeLink-compatible events
4. Publishes events to HomeLink via its HTTP API

## Quick Start

### Prerequisites

- Frigate NVR running and accessible via HTTP
- HomeLink service running and accessible via HTTP
- Go 1.21+ (for building from source) or Docker

### Using Docker Compose (Recommended)

1. Create a `docker-compose.yml` file:

```yaml
version: '3.8'
services:
  frigate-bridge:
    build: .
    environment:
      - FRIGATE_URL=http://frigate:5000
      - HOMELINK_URL=http://homelink:8081
      - HOMELINK_API_KEY=your-api-key
      - POLL_INTERVAL=5s
      - MIN_CONFIDENCE=0.8
    ports:
      - "8082:8082"  # Health check port
    restart: unless-stopped
    depends_on:
      - frigate
      - homelink
```

2. Run: `docker-compose up -d`

### Using Docker

```bash
docker build -t frigate-bridge .
docker run -d \
  --name frigate-bridge \
  -e FRIGATE_URL=http://your-frigate:5000 \
  -e HOMELINK_URL=http://your-homelink:8081 \
  -e HOMELINK_API_KEY=your-api-key \
  -p 8082:8082 \
  frigate-bridge
```

### Building from Source

```bash
git clone <repository>
cd frigate-bridge
go mod download
go build -o frigate-bridge .
./frigate-bridge
```

## Configuration

Configuration is handled via environment variables. Copy `.env.example` to `.env` and modify as needed.

### Required Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `FRIGATE_URL` | Frigate base URL | `http://localhost:5000` |
| `HOMELINK_URL` | HomeLink base URL | `http://localhost:8081` |

### Optional Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `HOMELINK_API_KEY` | HomeLink API key (if authentication enabled) | _(empty)_ |
| `BRIDGE_ID` | Unique identifier for this bridge instance | `frigate-bridge` |
| `POLL_INTERVAL` | How often to poll Frigate for new events | `5s` |
| `ENABLED_CAMERAS` | Comma-separated list of cameras to monitor | _(all cameras)_ |
| `ENABLED_EVENT_TYPES` | Comma-separated list of object types to forward | `person,car,bicycle,dog,cat` |
| `MIN_CONFIDENCE` | Minimum confidence threshold (0.0-1.0) | `0.7` |
| `MAX_EVENT_AGE` | Maximum age of events to process | `10m` |
| `RETRY_ATTEMPTS` | Number of retry attempts for failed publishing | `3` |
| `RETRY_DELAY` | Delay between retry attempts | `5s` |
| `HEALTH_CHECK_PORT` | Port for health check HTTP server | `8082` |

## Event Types

The bridge transforms Frigate detection events into HomeLink events:

| Frigate Label | HomeLink Event Type | Priority |
|---------------|-------------------|----------|
| `person` | `person_detected` | High (if confidence > 90%) |
| `car`, `truck`, `bus` | `vehicle_detected` | High (if confidence > 80%) |
| `bicycle`, `motorcycle` | `bicycle_detected` | Normal |
| `dog`, `cat` | `pet_detected` | Normal |
| `bird` | `bird_detected` | Normal |
| Others | `object_detected` | Normal |

High-priority events use HomeLink's reliable delivery mechanism for guaranteed delivery.

## HomeLink Event Data

Each forwarded event includes rich metadata:

```json
{
  "event_type": "person_detected",
  "description": "Person detected on front_door with 95% confidence",
  "data": {
    "source": "frigate",
    "bridge_id": "frigate-bridge",
    "frigate_id": "1704067200.123456-abc123",
    "camera": "front_door",
    "label": "person",
    "confidence": "0.95",
    "start_time": "1704067200.123",
    "has_snapshot": "true",
    "has_clip": "true",
    "zones": "entry,porch",
    "box_x": "100",
    "box_y": "50",
    "box_width": "200",
    "box_height": "300"
  }
}
```

## Monitoring

### Health Checks

The bridge provides a health check endpoint at `http://localhost:8082/health`:

```json
{
  "status": "healthy",
  "service": "frigate-homelink-bridge"
}
```

### Statistics

View detailed statistics at `http://localhost:8082/stats`:

```json
{
  "start_time": "2024-01-01T10:00:00Z",
  "uptime": "2h30m15s",
  "health_status": "healthy",
  "events_processed": 147,
  "events_filtered": 23,
  "events_published": 124,
  "events_failed": 0,
  "frigate_errors": 1,
  "homelink_errors": 0
}
```

## Filtering Examples

### Monitor Specific Cameras
```bash
ENABLED_CAMERAS=front_door,back_yard,driveway
```

### Only Person and Vehicle Detection
```bash
ENABLED_EVENT_TYPES=person,car,truck
```

### High Confidence Events Only
```bash
MIN_CONFIDENCE=0.9
```

## Troubleshooting

### Common Issues

1. **No events being processed**
   - Check Frigate URL and API accessibility
   - Verify event types match your Frigate configuration
   - Check confidence threshold isn't too high

2. **Events not appearing in HomeLink**
   - Verify HomeLink URL and API key
   - Check HomeLink logs for event processing
   - Test HomeLink API manually: `curl -X POST http://homelink:8081/publish`

3. **High memory usage**
   - Reduce `EVENT_BUFFER` size
   - Decrease `MAX_EVENT_AGE`
   - Increase `POLL_INTERVAL` for less frequent polling

### Logs

The service logs to stdout. Key log messages:

```
INFO: Starting Frigate-HomeLink Bridge Service
INFO: Polling Frigate for new events since 2024-01-01T10:00:00Z
INFO: Successfully processed and published event: abc123 (person_detected)
ERROR: Failed to publish event to HomeLink: connection refused
```

### Testing

Test the bridge components individually:

```bash
# Test Frigate connectivity
curl http://frigate:5000/api/version

# Test HomeLink connectivity
curl http://homelink:8081/health

# Test event publishing to HomeLink
curl -X POST http://homelink:8081/publish \
  -H "Content-Type: application/json" \
  -d '{"event_type":"test","description":"Bridge test"}'
```

## Development

### Building
```bash
go build -o frigate-bridge .
```

### Running Tests
```bash
go test ./...
```

### Adding Features
The bridge is designed to be extensible:
- Add new event transformations in `transformer.go`
- Extend filtering logic in `transformer.go`
- Add new health checks in `bridge_service.go`

## License

This project is open source. See LICENSE file for details.

## Support

For issues and questions:
1. Check the troubleshooting section
2. Review logs for error messages
3. Open an issue with your configuration and logs