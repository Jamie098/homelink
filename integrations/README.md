# Simple Integration Examples for HomeLink

This directory contains integration examples showing how to connect HomeLink with various systems.

## Frigate Integration

### Option 1: Webhook Integration

Use `frigate_webhook.sh` to handle Frigate webhook events:

1. Make the script executable:

   ```bash
   chmod +x frigate_webhook.sh
   ```

2. Configure Frigate to call this script as a webhook:

   ```yaml
   # In your Frigate config.yml
   mqtt:
     enabled: true

   objects:
     track:
       - person
       - car
       - truck

   cameras:
     front_door:
       # ... camera config
       objects:
         filters:
           person:
             min_area: 5000
             max_area: 100000
             threshold: 0.7
   ```

3. Set up a webhook endpoint that calls the script

### Option 2: MQTT Bridge

Use `frigate_mqtt_bridge.py` to listen to Frigate MQTT events:

1. Install requirements:

   ```bash
   pip install paho-mqtt requests
   ```

2. Configure the script variables at the top of the file:

   - `MQTT_BROKER`: Your MQTT broker address
   - `HOMELINK_API_URL`: HomeLink service URL

3. Run the bridge:
   ```bash
   python3 frigate_mqtt_bridge.py
   ```

## Generic Integration

For any system, you can send events to HomeLink using the HTTP API:

```bash
# Send a motion detection event
curl -X POST http://localhost:8081/publish \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "motion_detected",
    "description": "Motion detected at front door",
    "data": {
      "camera": "front_door",
      "confidence": "95",
      "zone": "entrance"
    }
  }'
```

## Environment Configuration

The HomeLink service can be configured using environment variables:

- `HOMELINK_DEVICE_ID`: Unique identifier for this device
- `HOMELINK_DEVICE_NAME`: Human-readable name
- `HOMELINK_CAPABILITIES`: Comma-separated list of capabilities
- `HOMELINK_API_PORT`: Port for HTTP API (default: 8081)

Example with Docker:

```bash
docker run -d \
  -e HOMELINK_DEVICE_ID=frigate-nvr \
  -e HOMELINK_DEVICE_NAME="Frigate Security System" \
  -e HOMELINK_CAPABILITIES=motion_detection,person_detection,vehicle_detection \
  -p 8080:8080 -p 8081:8081 \
  homelink:latest
```
