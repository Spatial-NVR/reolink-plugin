# Reolink Plugin for SpatialNVR

A plugin for integrating Reolink cameras and NVRs with SpatialNVR.

## Features

- Automatic camera detection and capability probing
- Support for single cameras and NVRs with multiple channels
- PTZ control for supported cameras
- AI detection events (person, vehicle, animal, face, package)
- RTSP and RTMP stream URLs
- Snapshot capture
- Two-way audio support

## Supported Devices

- Reolink IP cameras (RLC series, E1 series, etc.)
- Reolink NVRs (RLN series)
- Reolink doorbells
- Reolink battery cameras (Argus, Lumus)
- Reolink PTZ cameras (TrackMix, etc.)
- Reolink floodlight cameras

## Installation

### Via SpatialNVR UI

1. Open SpatialNVR web interface
2. Navigate to Settings > Plugins
3. Click "Add Plugin"
4. Enter repository URL: `github.com/Spatial-NVR/reolink-plugin`
5. Click Install

### Manual Installation

1. Download the latest release from GitHub Releases

2. Extract to the plugins directory:
   ```bash
   mkdir -p /data/plugins/reolink
   tar -xzf reolink-plugin-*.tar.gz -C /data/plugins/reolink/
   ```

3. Restart SpatialNVR

### Building from Source

```bash
# Clone the repository
git clone https://github.com/Spatial-NVR/reolink-plugin.git
cd reolink-plugin

# Build the plugin
go build -o reolink-plugin .

# Copy to plugins directory
mkdir -p /data/plugins/reolink
cp reolink-plugin manifest.yaml /data/plugins/reolink/
```

## Configuration

### Via Web UI

1. Navigate to Settings > Plugins > Reolink
2. Click "Add Device"
3. Enter the camera IP, username, and password
4. Click "Probe" to detect capabilities
5. Click "Add Camera"

### Via Config File

Add to your `config.yaml` under the `plugins` section:

```yaml
plugins:
  reolink:
    enabled: true
    config:
      devices:
        - host: 192.168.1.100
          username: admin
          password: your_password
          name: Front Door Camera
        - host: 192.168.1.101
          username: admin
          password: your_password
          channels: [0, 1, 2, 3]  # For NVRs with multiple channels
          name: Backyard NVR
```

## API Reference

### Plugin RPC Methods

The plugin implements the SpatialNVR plugin interface:

| Method | Description |
|--------|-------------|
| `initialize` | Initialize with configuration |
| `shutdown` | Graceful shutdown |
| `health` | Get plugin health status |
| `discover_cameras` | Scan network for Reolink devices |
| `add_camera` | Add a camera by credentials |
| `remove_camera` | Remove a camera |
| `list_cameras` | List all configured cameras |
| `get_camera` | Get camera details and status |
| `ptz_control` | Send PTZ commands |
| `get_snapshot` | Capture a snapshot |
| `probe_camera` | Probe camera for capabilities |

### Probing a Camera

Before adding a camera, probe it to detect capabilities:

```bash
curl -X POST http://localhost:12000/api/v1/plugins/reolink/probe \
  -H "Content-Type: application/json" \
  -d '{
    "host": "192.168.1.100",
    "username": "admin",
    "password": "your_password"
  }'
```

Response:
```json
{
  "host": "192.168.1.100",
  "model": "RLC-810A",
  "name": "Front Door",
  "device_type": "camera",
  "has_ptz": false,
  "has_two_way_audio": true,
  "has_ai_detection": true,
  "channel_count": 1,
  "channels": [
    {
      "channel": 0,
      "rtsp_main": "rtsp://admin:pass@192.168.1.100:554/h264Preview_01_main",
      "rtsp_sub": "rtsp://admin:pass@192.168.1.100:554/h264Preview_01_sub"
    }
  ]
}
```

### Adding a Camera

```bash
curl -X POST http://localhost:12000/api/v1/plugins/reolink/cameras \
  -H "Content-Type: application/json" \
  -d '{
    "host": "192.168.1.100",
    "username": "admin",
    "password": "your_password",
    "name": "Front Door"
  }'
```

### PTZ Control

For PTZ-capable cameras:

```bash
curl -X POST http://localhost:12000/api/v1/plugins/reolink/cameras/{camera_id}/ptz \
  -H "Content-Type: application/json" \
  -d '{
    "command": "right",
    "speed": 50
  }'
```

Commands: `up`, `down`, `left`, `right`, `zoom_in`, `zoom_out`, `stop`

### Get Snapshot

```bash
curl http://localhost:12000/api/v1/plugins/reolink/cameras/{camera_id}/snapshot \
  -o snapshot.jpg
```

## Stream URLs

The plugin generates stream URLs in the format expected by go2rtc:

- **Main stream (HD)**: `rtsp://user:pass@host:554/h264Preview_01_main`
- **Sub stream (SD)**: `rtsp://user:pass@host:554/h264Preview_01_sub`

For NVRs with multiple channels, the channel number is embedded in the URL:
- Channel 0: `h264Preview_01_main`
- Channel 1: `h264Preview_02_main`

## Troubleshooting

### Camera Not Responding

1. Verify the camera is reachable: `ping 192.168.1.100`
2. Check credentials in the Reolink app
3. Ensure the camera firmware is up to date
4. Check if the camera's HTTP port (default 80) is accessible

### Stream Not Playing

1. Check if RTSP is enabled on the camera
2. Verify the RTSP port (default 554) is not blocked
3. Try the sub-stream if main stream is too high bandwidth
4. Check go2rtc logs for connection errors

### PTZ Not Working

1. Verify the camera supports PTZ
2. Check if PTZ is enabled in camera settings
3. Some cameras require specific firmware for PTZ API

## Development

### Running Tests

```bash
go test -v ./...
```

### Building for Different Platforms

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o reolink-plugin-linux-amd64 .

# Linux ARM64 (Raspberry Pi 4)
GOOS=linux GOARCH=arm64 go build -o reolink-plugin-linux-arm64 .

# macOS
GOOS=darwin GOARCH=arm64 go build -o reolink-plugin-darwin-arm64 .
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests
5. Submit a pull request

See the main [SpatialNVR Contributing Guide](https://github.com/Spatial-NVR/SpatialNVR/blob/main/docs/CONTRIBUTING.md) for style guidelines.
