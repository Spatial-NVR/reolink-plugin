// Reolink Plugin for NVR System
// This is a standalone plugin that communicates via JSON-RPC over stdin/stdout
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Reolink plugin starting...")

	plugin := NewPlugin()

	// Read JSON-RPC requests from stdin, write responses to stdout
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Failed to parse request: %v", err)
			continue
		}

		resp := plugin.HandleRequest(req)
		respBytes, _ := json.Marshal(resp)
		fmt.Println(string(respBytes))
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}

	log.Println("Reolink plugin shutting down...")
}

// JSON-RPC types
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Plugin types
type Plugin struct {
	cameras map[string]*Camera
	devices []DeviceConfig
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

type DeviceConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
	Channels []int  `json:"channels,omitempty"`
	Name     string `json:"name,omitempty"`
}

type CameraConfig struct {
	Host     string                 `json:"host"`
	Port     int                    `json:"port,omitempty"`
	Username string                 `json:"username"`
	Password string                 `json:"password"`
	Channel  int                    `json:"channel,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Protocol string                 `json:"protocol,omitempty"` // "hls" (default), "rtsp", or "rtmp"
	Extra    map[string]interface{} `json:"extra,omitempty"`
}

type PluginCamera struct {
	ID           string   `json:"id"`
	PluginID     string   `json:"plugin_id"`
	Name         string   `json:"name"`
	Model        string   `json:"model"`
	Host         string   `json:"host"`
	MainStream   string   `json:"main_stream"`
	SubStream    string   `json:"sub_stream"`
	SnapshotURL  string   `json:"snapshot_url"`
	Capabilities []string `json:"capabilities"`
	Online       bool     `json:"online"`
	LastSeen     string   `json:"last_seen"`
	Protocol     string   `json:"protocol"` // "hls", "rtsp", or "rtmp"
}

type DiscoveredCamera struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Model           string   `json:"model"`
	Manufacturer    string   `json:"manufacturer"`
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	Channels        int      `json:"channels"`
	Capabilities    []string `json:"capabilities"`
	FirmwareVersion string   `json:"firmware_version,omitempty"`
	Serial          string   `json:"serial,omitempty"`
}

type HealthStatus struct {
	State     string                 `json:"state"`
	Message   string                 `json:"message,omitempty"`
	LastCheck string                 `json:"last_check"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type PTZCommand struct {
	Action    string  `json:"action"`
	Direction float64 `json:"direction,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
	Preset    string  `json:"preset,omitempty"`
}

func NewPlugin() *Plugin {
	return &Plugin{
		cameras: make(map[string]*Camera),
	}
}

func (p *Plugin) HandleRequest(req JSONRPCRequest) JSONRPCResponse {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	ctx := context.Background()
	if p.ctx != nil {
		ctx = p.ctx
	}

	switch req.Method {
	case "initialize":
		var config map[string]interface{}
		if req.Params != nil {
			_ = json.Unmarshal(req.Params, &config)
		}
		if err := p.Initialize(ctx, config); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = map[string]interface{}{"status": "ok"}
		}

	case "shutdown":
		if err := p.Shutdown(ctx); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = map[string]interface{}{"status": "ok"}
		}

	case "health":
		resp.Result = p.Health()

	case "discover_cameras":
		cameras, err := p.DiscoverCameras(ctx)
		if err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = cameras
		}

	case "add_camera":
		var config CameraConfig
		if err := json.Unmarshal(req.Params, &config); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params: " + err.Error()}
		} else {
			cam, err := p.AddCamera(ctx, config)
			if err != nil {
				resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
			} else {
				resp.Result = cam
			}
		}

	case "remove_camera":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if err := p.RemoveCamera(ctx, params.CameraID); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = map[string]interface{}{"status": "ok"}
		}

	case "list_cameras":
		resp.Result = p.ListCameras()

	case "get_camera":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if cam := p.GetCamera(params.CameraID); cam != nil {
			resp.Result = cam
		} else {
			resp.Error = &JSONRPCError{Code: -32603, Message: "Camera not found"}
		}

	case "update_camera":
		var params struct {
			CameraID string                 `json:"camera_id"`
			Settings map[string]interface{} `json:"settings"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params: " + err.Error()}
		} else if err := p.UpdateCamera(params.CameraID, params.Settings); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			// Return updated camera info
			resp.Result = p.GetCamera(params.CameraID)
		}

	case "ptz_control":
		var params struct {
			CameraID string     `json:"camera_id"`
			Command  PTZCommand `json:"command"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if err := p.PTZControl(ctx, params.CameraID, params.Command); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = map[string]interface{}{"status": "ok"}
		}

	case "get_snapshot":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if data, err := p.GetSnapshot(ctx, params.CameraID); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = data // base64 encoded
		}

	case "probe_camera":
		var params struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else {
			result, err := p.ProbeCamera(ctx, params.Host, params.Port, params.Username, params.Password)
			if err != nil {
				resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
			} else {
				resp.Result = result
			}
		}

	case "get_capabilities":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if caps := p.GetCapabilities(params.CameraID); caps != nil {
			resp.Result = caps
		} else {
			resp.Error = &JSONRPCError{Code: -32603, Message: "Camera not found"}
		}

	case "get_ptz_presets":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else {
			presets, err := p.GetPTZPresets(ctx, params.CameraID)
			if err != nil {
				resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
			} else {
				resp.Result = presets
			}
		}

	case "get_protocols":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if protocols := p.GetProtocols(params.CameraID); protocols != nil {
			resp.Result = protocols
		} else {
			resp.Error = &JSONRPCError{Code: -32603, Message: "Camera not found"}
		}

	case "set_protocol":
		var params struct {
			CameraID string `json:"camera_id"`
			Protocol string `json:"protocol"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if err := p.SetProtocol(params.CameraID, params.Protocol); err != nil {
			resp.Error = &JSONRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = p.GetCamera(params.CameraID)
		}

	case "get_device_info":
		var params struct {
			CameraID string `json:"camera_id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
		} else if info := p.GetDeviceInfo(params.CameraID); info != nil {
			resp.Result = info
		} else {
			resp.Error = &JSONRPCError{Code: -32603, Message: "Camera not found"}
		}

	default:
		resp.Error = &JSONRPCError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	return resp
}

func (p *Plugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	if err := p.parseConfig(config); err != nil {
		return err
	}

	// Connect to configured devices
	for _, device := range p.devices {
		if err := p.connectDevice(device); err != nil {
			log.Printf("Failed to connect to device %s: %v", device.Host, err)
		}
	}

	log.Printf("Plugin initialized with %d devices", len(p.devices))
	return nil
}

func (p *Plugin) parseConfig(config map[string]interface{}) error {
	p.devices = nil

	if config == nil {
		return nil
	}

	// Look for "devices" array
	if devicesRaw, ok := config["devices"]; ok {
		if devicesList, ok := devicesRaw.([]interface{}); ok {
			for _, d := range devicesList {
				if deviceMap, ok := d.(map[string]interface{}); ok {
					device := DeviceConfig{}
					if host, ok := deviceMap["host"].(string); ok {
						device.Host = host
					}
					if port, ok := deviceMap["port"].(float64); ok {
						device.Port = int(port)
					}
					if user, ok := deviceMap["username"].(string); ok {
						device.Username = user
					}
					if pass, ok := deviceMap["password"].(string); ok {
						device.Password = pass
					}
					if name, ok := deviceMap["name"].(string); ok {
						device.Name = name
					}
					if device.Host != "" {
						p.devices = append(p.devices, device)
					}
				}
			}
		}
	}

	return nil
}

func (p *Plugin) connectDevice(device DeviceConfig) error {
	client := NewClient(device.Host, device.Port, device.Username, device.Password)

	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	if err := client.Login(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	info, err := client.GetDeviceInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	log.Printf("Connected to %s (%s) with %d channels", info.Name, info.Model, info.ChannelCount)

	ability, _ := client.GetAbility(ctx, 0)

	channels := device.Channels
	if len(channels) == 0 {
		for i := 0; i < info.ChannelCount; i++ {
			channels = append(channels, i)
		}
	}

	for _, ch := range channels {
		cameraID := fmt.Sprintf("%s_ch%d", device.Host, ch)
		cameraName := info.Name
		if device.Name != "" {
			cameraName = device.Name
		}
		if info.ChannelCount > 1 {
			cameraName = fmt.Sprintf("%s Ch%d", cameraName, ch+1)
		}

		cam := NewCamera(cameraID, cameraName, info.Model, device.Host, ch, client)
		if ability != nil {
			cam.SetAbility(ability)
		}

		p.mu.Lock()
		p.cameras[cameraID] = cam
		p.mu.Unlock()

		log.Printf("Added camera: %s", cameraID)
	}

	return nil
}

func (p *Plugin) Shutdown(ctx context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	log.Println("Plugin shutdown complete")
	return nil
}

func (p *Plugin) Health() HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	online := 0
	total := len(p.cameras)

	for _, cam := range p.cameras {
		if cam.IsOnline() {
			online++
		}
	}

	state := "healthy"
	msg := fmt.Sprintf("%d/%d cameras online", online, total)

	if total == 0 {
		state = "unknown"
		msg = "No cameras configured"
	} else if online == 0 {
		state = "unhealthy"
	} else if online < total {
		state = "degraded"
	}

	return HealthStatus{
		State:     state,
		Message:   msg,
		LastCheck: time.Now().Format(time.RFC3339),
		Details: map[string]interface{}{
			"cameras_online": online,
			"cameras_total":  total,
		},
	}
}

func (p *Plugin) DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var discovered []DiscoveredCamera
	for _, cam := range p.cameras {
		discovered = append(discovered, DiscoveredCamera{
			ID:           cam.ID(),
			Name:         cam.Name(),
			Model:        cam.Model(),
			Manufacturer: "Reolink",
			Host:         cam.Host(),
			Capabilities: cam.Capabilities(),
		})
	}

	return discovered, nil
}

func (p *Plugin) AddCamera(ctx context.Context, cfg CameraConfig) (*PluginCamera, error) {
	device := DeviceConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		Name:     cfg.Name,
	}

	if cfg.Channel > 0 {
		device.Channels = []int{cfg.Channel}
	}

	if err := p.connectDevice(device); err != nil {
		return nil, err
	}

	cameraID := fmt.Sprintf("%s_ch%d", cfg.Host, cfg.Channel)

	// Apply protocol setting if specified
	if cfg.Protocol != "" {
		p.mu.RLock()
		if cam, ok := p.cameras[cameraID]; ok {
			cam.SetProtocol(cfg.Protocol)
		}
		p.mu.RUnlock()
	}

	return p.GetCamera(cameraID), nil
}

func (p *Plugin) RemoveCamera(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.cameras[id]; !ok {
		return fmt.Errorf("camera not found: %s", id)
	}

	delete(p.cameras, id)
	log.Printf("Removed camera: %s", id)
	return nil
}

func (p *Plugin) ListCameras() []PluginCamera {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cameras := make([]PluginCamera, 0, len(p.cameras))
	for _, cam := range p.cameras {
		cameras = append(cameras, PluginCamera{
			ID:           cam.ID(),
			PluginID:     "reolink",
			Name:         cam.Name(),
			Model:        cam.Model(),
			Host:         cam.Host(),
			MainStream:   cam.StreamURL("main"),
			SubStream:    cam.StreamURL("sub"),
			SnapshotURL:  cam.SnapshotURL(),
			Capabilities: cam.Capabilities(),
			Online:       cam.IsOnline(),
			LastSeen:     cam.LastSeen().Format(time.RFC3339),
			Protocol:     cam.Protocol(),
		})
	}
	return cameras
}

func (p *Plugin) GetCamera(id string) *PluginCamera {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cam, ok := p.cameras[id]
	if !ok {
		return nil
	}

	return &PluginCamera{
		ID:           cam.ID(),
		PluginID:     "reolink",
		Name:         cam.Name(),
		Model:        cam.Model(),
		Host:         cam.Host(),
		MainStream:   cam.StreamURL("main"),
		SubStream:    cam.StreamURL("sub"),
		SnapshotURL:  cam.SnapshotURL(),
		Capabilities: cam.Capabilities(),
		Online:       cam.IsOnline(),
		LastSeen:     cam.LastSeen().Format(time.RFC3339),
		Protocol:     cam.Protocol(),
	}
}

// UpdateCamera updates camera settings (like protocol)
func (p *Plugin) UpdateCamera(id string, settings map[string]interface{}) error {
	p.mu.RLock()
	cam, ok := p.cameras[id]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("camera not found: %s", id)
	}

	if protocol, ok := settings["protocol"].(string); ok {
		cam.SetProtocol(protocol)
		log.Printf("Updated camera %s protocol to %s", id, protocol)
	}

	return nil
}

func (p *Plugin) PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error {
	p.mu.RLock()
	cam, ok := p.cameras[cameraID]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	return cam.PTZControl(ctx, cmd)
}

func (p *Plugin) GetSnapshot(ctx context.Context, cameraID string) (string, error) {
	p.mu.RLock()
	cam, ok := p.cameras[cameraID]
	p.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("camera not found: %s", cameraID)
	}

	return cam.GetSnapshot(ctx)
}

func (p *Plugin) ProbeCamera(ctx context.Context, host string, port int, username, password string) (*CameraProbeResult, error) {
	if port == 0 {
		port = 80
	}
	client := NewClient(host, port, username, password)
	return client.ProbeCamera(ctx)
}

// CameraCapabilities represents detailed capabilities for a camera
type CameraCapabilities struct {
	HasPTZ          bool     `json:"has_ptz"`
	HasAudio        bool     `json:"has_audio"`
	HasTwoWayAudio  bool     `json:"has_two_way_audio"`
	HasSnapshot     bool     `json:"has_snapshot"`
	DeviceType      string   `json:"device_type"`
	IsDoorbell      bool     `json:"is_doorbell"`
	IsNVR           bool     `json:"is_nvr"`
	IsBattery       bool     `json:"is_battery"`
	HasAIDetection  bool     `json:"has_ai_detection"`
	AITypes         []string `json:"ai_types,omitempty"`
	Protocols       []string `json:"protocols"`
	CurrentProtocol string   `json:"current_protocol"`
}

// ProtocolOption represents an available streaming protocol
type ProtocolOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	StreamURL   string `json:"stream_url,omitempty"`
}

// PTZPreset represents a saved PTZ position
type PTZPreset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RPCDeviceInfo represents detailed device information for RPC responses
type RPCDeviceInfo struct {
	Model           string `json:"model"`
	Manufacturer    string `json:"manufacturer"`
	Serial          string `json:"serial,omitempty"`
	FirmwareVersion string `json:"firmware_version,omitempty"`
	HardwareVersion string `json:"hardware_version,omitempty"`
	ChannelCount    int    `json:"channel_count"`
	DeviceType      string `json:"device_type,omitempty"`
}

// GetCapabilities returns detailed capabilities for a camera
func (p *Plugin) GetCapabilities(cameraID string) *CameraCapabilities {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cam, ok := p.cameras[cameraID]
	if !ok {
		return nil
	}

	caps := cam.Capabilities()
	hasPTZ := contains(caps, "ptz")
	hasAudio := contains(caps, "audio")
	hasTwoWay := contains(caps, "two_way_audio")
	hasAI := contains(caps, "ai_detection")

	// Determine AI types based on capabilities
	var aiTypes []string
	if hasAI {
		aiTypes = []string{"person", "vehicle", "animal"}
	}

	return &CameraCapabilities{
		HasPTZ:          hasPTZ,
		HasAudio:        hasAudio,
		HasTwoWayAudio:  hasTwoWay,
		HasSnapshot:     true,
		DeviceType:      cam.DeviceType(),
		IsDoorbell:      cam.DeviceType() == "doorbell",
		IsNVR:           cam.DeviceType() == "nvr",
		IsBattery:       cam.DeviceType() == "battery",
		HasAIDetection:  hasAI,
		AITypes:         aiTypes,
		Protocols:       []string{"rtsp", "rtmp", "hls"},
		CurrentProtocol: cam.Protocol(),
	}
}

// GetPTZPresets returns available PTZ presets for a camera
func (p *Plugin) GetPTZPresets(ctx context.Context, cameraID string) ([]PTZPreset, error) {
	p.mu.RLock()
	cam, ok := p.cameras[cameraID]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("camera not found: %s", cameraID)
	}

	// Get presets from camera
	presets, err := cam.GetPTZPresets(ctx)
	if err != nil {
		return nil, err
	}

	var result []PTZPreset
	for _, preset := range presets {
		result = append(result, PTZPreset{
			ID:   preset.ID,
			Name: preset.Name,
		})
	}

	return result, nil
}

// GetProtocols returns available streaming protocols for a camera
func (p *Plugin) GetProtocols(cameraID string) []ProtocolOption {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cam, ok := p.cameras[cameraID]
	if !ok {
		return nil
	}

	return []ProtocolOption{
		{
			ID:          "rtsp",
			Name:        "RTSP",
			Description: "Real Time Streaming Protocol - low latency",
			StreamURL:   cam.StreamURLForProtocol("main", "rtsp"),
		},
		{
			ID:          "rtmp",
			Name:        "RTMP",
			Description: "Real Time Messaging Protocol - compatible",
			StreamURL:   cam.StreamURLForProtocol("main", "rtmp"),
		},
		{
			ID:          "hls",
			Name:        "HLS/HTTP-FLV",
			Description: "HTTP streaming - firewall friendly",
			StreamURL:   cam.StreamURLForProtocol("main", "hls"),
		},
	}
}

// SetProtocol changes the streaming protocol for a camera
func (p *Plugin) SetProtocol(cameraID string, protocol string) error {
	p.mu.RLock()
	cam, ok := p.cameras[cameraID]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	// Validate protocol
	validProtocols := map[string]bool{"rtsp": true, "rtmp": true, "hls": true}
	if !validProtocols[protocol] {
		return fmt.Errorf("invalid protocol: %s (must be rtsp, rtmp, or hls)", protocol)
	}

	cam.SetProtocol(protocol)
	log.Printf("Set camera %s protocol to %s", cameraID, protocol)
	return nil
}

// GetDeviceInfo returns detailed device information for a camera
func (p *Plugin) GetDeviceInfo(cameraID string) *RPCDeviceInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cam, ok := p.cameras[cameraID]
	if !ok {
		return nil
	}

	info := cam.GetDeviceInfo()
	if info == nil {
		return &RPCDeviceInfo{
			Model:        cam.Model(),
			Manufacturer: "Reolink",
			ChannelCount: 1,
			DeviceType:   cam.DeviceType(),
		}
	}

	return &RPCDeviceInfo{
		Model:           info.Model,
		Manufacturer:    "Reolink",
		Serial:          info.Serial,
		FirmwareVersion: info.FirmwareVersion,
		HardwareVersion: info.HardwareVersion,
		ChannelCount:    info.ChannelCount,
		DeviceType:      cam.DeviceType(),
	}
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
