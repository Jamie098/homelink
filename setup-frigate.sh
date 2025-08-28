#!/bin/bash
set -e

echo "🏠 HomeLink + Frigate Docker Setup"
echo "=================================="

# Create required directories
echo "📁 Creating directories..."
mkdir -p frigate_config
mkdir -p frigate_storage
mkdir -p homelink_data
mkdir -p homelink_snapshots

# Copy the Frigate configuration
echo "📋 Setting up Frigate configuration..."
if [ -f "frigate-config.yml" ]; then
    cp frigate-config.yml frigate_config/config.yml
    echo "✅ Frigate config copied to frigate_config/config.yml"
    echo "⚠️  Please edit frigate_config/config.yml with your camera details!"
else
    echo "❌ frigate-config.yml not found. Please create it first."
    exit 1
fi

# Check if Docker Compose file exists
if [ ! -f "docker-compose.yml" ]; then
    echo "❌ docker-compose.yml not found!"
    exit 1
fi

echo ""
echo "🔧 Configuration checklist:"
echo "1. Edit frigate_config/config.yml with your camera RTSP URLs"
echo "2. Update webhook secret in both docker-compose.yml and frigate_config/config.yml"
echo "3. Optionally enable HomeLink security/storage in docker-compose.yml"
echo ""

read -p "Have you configured your cameras in frigate_config/config.yml? (y/n): " -r
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Please configure your cameras first, then run this script again."
    echo "Edit: frigate_config/config.yml"
    exit 1
fi

echo ""
echo "🚀 Starting HomeLink + Frigate..."
echo ""

# Build and start services
docker-compose up -d --build

echo ""
echo "✅ Services started! Checking health..."
sleep 10

# Check service status
echo ""
echo "📊 Service Status:"
docker-compose ps

echo ""
echo "🔗 Access URLs:"
echo "  - HomeLink API:    http://localhost:8081"
echo "  - HomeLink Dashboard: http://localhost:8081/dashboard" 
echo "  - Frigate UI:      http://localhost:5000"
echo "  - Frigate API:     http://localhost:5000/api/version"
echo ""

# Test HomeLink health
echo "🏥 Testing HomeLink health..."
if curl -s http://localhost:8081/health | grep -q "success"; then
    echo "✅ HomeLink is healthy"
else
    echo "❌ HomeLink health check failed"
fi

# Test Frigate health  
echo "🏥 Testing Frigate health..."
if curl -s http://localhost:5000/api/version | grep -q "version"; then
    echo "✅ Frigate is healthy"
else
    echo "❌ Frigate health check failed"
fi

# Test Frigate integration
echo "🔗 Testing Frigate integration..."
if curl -s http://localhost:8081/frigate/stats | grep -q "enabled"; then
    echo "✅ Frigate integration is enabled"
else
    echo "❌ Frigate integration check failed"
fi

echo ""
echo "🎉 Setup complete!"
echo ""
echo "📱 For Android app integration:"
echo "  - Subscribe to events: frigate_new, frigate_update, frigate_end"
echo "  - Use HomeLink discovery to find the service"
echo "  - HomeLink device ID: homelink-docker"
echo ""
echo "🔧 To customize settings:"
echo "  - Edit docker-compose.yml for HomeLink settings"  
echo "  - Edit frigate_config/config.yml for camera settings"
echo "  - Restart: docker-compose restart"
echo ""
echo "📋 View logs:"
echo "  - HomeLink: docker-compose logs -f homelink"
echo "  - Frigate:  docker-compose logs -f frigate"
echo "  - All:      docker-compose logs -f"