# HomeLink + Frigate Integration Guide

## 🚀 Quick Start

1. **Configure your cameras:**
   ```bash
   # Edit frigate-config.yml with your camera RTSP URLs
   vim frigate-config.yml
   ```

2. **Run the setup:**
   ```bash
   ./setup-frigate.sh
   ```

3. **Access the services:**
   - HomeLink API: http://localhost:8081
   - Frigate UI: http://localhost:5000

## 📱 Android App Integration

Your Android app should subscribe to these HomeLink events:
- `frigate_new` - New detection (person, car, etc.)
- `frigate_update` - Detection updated
- `frigate_end` - Detection ended

### Event Data Structure
```json
{
  "source": "frigate",
  "event_type": "new",
  "event_id": "1642534800.123456-abc123",
  "camera": "front_door",
  "label": "person",
  "score": "0.85",
  "has_snapshot": "true",
  "snapshot_url": "/frigate/snapshot/1642534800.123456-abc123",
  "zones": "front_yard,driveway",
  "box": "100,150,300,450",
  "timestamp": "1642534800"
}
```

### Notification Flow
1. **Frigate** detects motion → sends webhook to HomeLink
2. **HomeLink** processes event → caches snapshot → broadcasts to network
3. **Android app** receives HomeLink event → shows notification with snapshot

## 🔧 Configuration

### Environment Variables (docker-compose.yml)
```yaml
# Frigate Integration
- HOMELINK_FRIGATE_ENABLED=true
- HOMELINK_FRIGATE_BASE_URL=http://frigate:5000
- HOMELINK_FRIGATE_NOTIFY_EVENTS=new        # new,update,end
- HOMELINK_FRIGATE_NOTIFY_LABELS=person,car # Filter by object type
- HOMELINK_FRIGATE_MIN_SCORE=0.7            # Confidence threshold
- HOMELINK_FRIGATE_ZONE_FILTER=front_yard   # Specific zones only
- HOMELINK_FRIGATE_WEBHOOK_SECRET=secret    # Webhook authentication
```

### Camera Configuration (frigate-config.yml)
```yaml
cameras:
  front_door:
    ffmpeg:
      inputs:
        - path: rtsp://192.168.1.100:554/stream1
          roles: [detect, record]
    zones:
      front_yard:
        coordinates: 0,720,1280,720,1280,400,0,400
        objects: [person, car]
```

## 📊 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/frigate/webhook` | POST | Receives Frigate webhooks |
| `/frigate/snapshot/{event_id}` | GET | Serves cached snapshots |
| `/frigate/stats` | GET | Integration statistics |
| `/publish` | POST | Manual event publishing |
| `/devices` | GET | List HomeLink devices |

## 🔍 Testing

### Test webhook manually:
```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{"type":"new","camera":"front_door","label":"person","score":0.85}' \
  http://localhost:8081/frigate/webhook
```

### Check integration status:
```bash
curl http://localhost:8081/frigate/stats
```

### View logs:
```bash
docker-compose logs -f homelink
docker-compose logs -f frigate
```

## 🎛️ Frigate Webhook Configuration

The webhook is automatically configured in `frigate-config.yml`:
```yaml
webhooks:
  homelink:
    url: http://homelink:8081/frigate/webhook
    method: POST
    headers:
      Content-Type: application/json
      X-Frigate-Secret: your-webhook-secret-here
    events: [new, update, end]
```

## 🔐 Security Features

- **Webhook Secret**: Prevents unauthorized webhook calls
- **Snapshot Caching**: Local caching with configurable TTL
- **Event Filtering**: Multiple filter layers (confidence, zones, labels)
- **HomeLink Security**: Optional encryption and device authentication

## 📱 Mobile App Example

Subscribe to HomeLink events in your Android app:
```kotlin
homelinkService.subscribe(listOf("frigate_new", "frigate_update"))
```

Handle incoming events:
```kotlin
when (event.eventType) {
    "frigate_new" -> {
        val camera = event.data["camera"]
        val label = event.data["label"] 
        val snapshotUrl = "http://homelink:8081${event.data["snapshot_url"]}"
        showNotification("$label detected on $camera", snapshotUrl)
    }
}
```

## 🐛 Troubleshooting

### Common Issues:

1. **Frigate can't connect to cameras:**
   - Check RTSP URLs in `frigate_config/config.yml`
   - Verify camera credentials and network access

2. **HomeLink not receiving events:**
   - Check webhook secret matches in both configs
   - Verify Frigate webhook is configured correctly
   - Check `docker-compose logs frigate`

3. **Snapshots not loading:**
   - Ensure Frigate base URL is correct
   - Check if Frigate API is accessible from HomeLink container
   - Verify snapshot caching is enabled

4. **Android app not getting notifications:**
   - Confirm app is subscribed to `frigate_*` events
   - Check HomeLink device discovery
   - Verify network connectivity to HomeLink service

### Health Checks:
```bash
# HomeLink health
curl http://localhost:8081/health

# Frigate health  
curl http://localhost:5000/api/version

# Integration status
curl http://localhost:8081/frigate/stats
```

## 📋 Next Steps

1. Configure your camera RTSP URLs
2. Set up zones and object filters in Frigate
3. Test the webhook integration
4. Update your Android app to handle Frigate events
5. Customize notification priorities and filtering