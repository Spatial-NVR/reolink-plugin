package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewPlugin(t *testing.T) {
	plugin := NewPlugin()
	if plugin == nil {
		t.Fatal("NewPlugin returned nil")
	}
	if plugin.cameras == nil {
		t.Error("cameras map should be initialized")
	}
	if len(plugin.cameras) != 0 {
		t.Error("cameras map should be empty initially")
	}
}

func TestPlugin_Initialize_NilConfig(t *testing.T) {
	plugin := NewPlugin()
	ctx := context.Background()

	err := plugin.Initialize(ctx, nil)
	if err != nil {
		t.Errorf("Initialize with nil config should not error: %v", err)
	}
}

func TestPlugin_Initialize_EmptyConfig(t *testing.T) {
	plugin := NewPlugin()
	ctx := context.Background()

	err := plugin.Initialize(ctx, map[string]interface{}{})
	if err != nil {
		t.Errorf("Initialize with empty config should not error: %v", err)
	}
}

func TestPlugin_Shutdown(t *testing.T) {
	plugin := NewPlugin()
	ctx := context.Background()

	// Initialize first
	_ = plugin.Initialize(ctx, nil)

	// Then shutdown
	err := plugin.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should not error: %v", err)
	}
}

func TestPlugin_Health_NoCameras(t *testing.T) {
	plugin := NewPlugin()

	health := plugin.Health()

	if health.State != "unknown" {
		t.Errorf("Expected state 'unknown', got '%s'", health.State)
	}
	if health.Message != "No cameras configured" {
		t.Errorf("Unexpected message: %s", health.Message)
	}
	if health.LastCheck == "" {
		t.Error("LastCheck should be set")
	}
}

func TestPlugin_Health_WithOnlineCameras(t *testing.T) {
	plugin := NewPlugin()

	// Add mock cameras
	client := NewClient("localhost", 80, "admin", "password")
	cam1 := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	cam1.online = true
	cam2 := NewCamera("cam_2", "Back Yard", "RLC-810A", "localhost", 0, client)
	cam2.online = true

	plugin.cameras["cam_1"] = cam1
	plugin.cameras["cam_2"] = cam2

	health := plugin.Health()

	if health.State != "healthy" {
		t.Errorf("Expected state 'healthy', got '%s'", health.State)
	}

	details := health.Details
	if details["cameras_online"] != 2 {
		t.Errorf("Expected 2 cameras online, got %v", details["cameras_online"])
	}
	if details["cameras_total"] != 2 {
		t.Errorf("Expected 2 total cameras, got %v", details["cameras_total"])
	}
}

func TestPlugin_Health_WithOfflineCameras(t *testing.T) {
	plugin := NewPlugin()

	// Add mock cameras
	client := NewClient("localhost", 80, "admin", "password")
	cam1 := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	cam1.online = false
	cam2 := NewCamera("cam_2", "Back Yard", "RLC-810A", "localhost", 0, client)
	cam2.online = false

	plugin.cameras["cam_1"] = cam1
	plugin.cameras["cam_2"] = cam2

	health := plugin.Health()

	if health.State != "unhealthy" {
		t.Errorf("Expected state 'unhealthy', got '%s'", health.State)
	}
}

func TestPlugin_Health_Degraded(t *testing.T) {
	plugin := NewPlugin()

	// Add mock cameras - one online, one offline
	client := NewClient("localhost", 80, "admin", "password")
	cam1 := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	cam1.online = true
	cam2 := NewCamera("cam_2", "Back Yard", "RLC-810A", "localhost", 0, client)
	cam2.online = false

	plugin.cameras["cam_1"] = cam1
	plugin.cameras["cam_2"] = cam2

	health := plugin.Health()

	if health.State != "degraded" {
		t.Errorf("Expected state 'degraded', got '%s'", health.State)
	}
}

func TestPlugin_DiscoverCameras(t *testing.T) {
	plugin := NewPlugin()

	// Add mock cameras
	client := NewClient("localhost", 80, "admin", "password")
	cam := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	plugin.cameras["cam_1"] = cam

	ctx := context.Background()
	discovered, err := plugin.DiscoverCameras(ctx)

	if err != nil {
		t.Errorf("DiscoverCameras should not error: %v", err)
	}
	if len(discovered) != 1 {
		t.Errorf("Expected 1 discovered camera, got %d", len(discovered))
	}

	d := discovered[0]
	if d.ID != "cam_1" {
		t.Errorf("Expected ID 'cam_1', got '%s'", d.ID)
	}
	if d.Manufacturer != "Reolink" {
		t.Errorf("Expected manufacturer 'Reolink', got '%s'", d.Manufacturer)
	}
}

func TestPlugin_ListCameras(t *testing.T) {
	plugin := NewPlugin()

	// Add mock cameras
	client := NewClient("localhost", 80, "admin", "password")
	cam := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	plugin.cameras["cam_1"] = cam

	cameras := plugin.ListCameras()

	if len(cameras) != 1 {
		t.Errorf("Expected 1 camera, got %d", len(cameras))
	}

	c := cameras[0]
	if c.ID != "cam_1" {
		t.Errorf("Expected ID 'cam_1', got '%s'", c.ID)
	}
	if c.PluginID != "reolink" {
		t.Errorf("Expected PluginID 'reolink', got '%s'", c.PluginID)
	}
}

func TestPlugin_GetCamera_Found(t *testing.T) {
	plugin := NewPlugin()

	// Add mock camera
	client := NewClient("localhost", 80, "admin", "password")
	cam := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	plugin.cameras["cam_1"] = cam

	result := plugin.GetCamera("cam_1")

	if result == nil {
		t.Fatal("GetCamera should return camera")
	}
	if result.ID != "cam_1" {
		t.Errorf("Expected ID 'cam_1', got '%s'", result.ID)
	}
}

func TestPlugin_GetCamera_NotFound(t *testing.T) {
	plugin := NewPlugin()

	result := plugin.GetCamera("nonexistent")

	if result != nil {
		t.Error("GetCamera should return nil for nonexistent camera")
	}
}

func TestPlugin_RemoveCamera_Found(t *testing.T) {
	plugin := NewPlugin()

	// Add mock camera
	client := NewClient("localhost", 80, "admin", "password")
	cam := NewCamera("cam_1", "Front Door", "RLC-810A", "localhost", 0, client)
	plugin.cameras["cam_1"] = cam

	ctx := context.Background()
	err := plugin.RemoveCamera(ctx, "cam_1")

	if err != nil {
		t.Errorf("RemoveCamera should not error: %v", err)
	}

	if len(plugin.cameras) != 0 {
		t.Error("Camera should be removed")
	}
}

func TestPlugin_RemoveCamera_NotFound(t *testing.T) {
	plugin := NewPlugin()

	ctx := context.Background()
	err := plugin.RemoveCamera(ctx, "nonexistent")

	if err == nil {
		t.Error("RemoveCamera should error for nonexistent camera")
	}
}

func TestPlugin_PTZControl_CameraNotFound(t *testing.T) {
	plugin := NewPlugin()

	ctx := context.Background()
	cmd := PTZCommand{Action: "pan", Direction: 1}

	err := plugin.PTZControl(ctx, "nonexistent", cmd)

	if err == nil {
		t.Error("PTZControl should error for nonexistent camera")
	}
}

func TestPlugin_GetSnapshot_CameraNotFound(t *testing.T) {
	plugin := NewPlugin()

	ctx := context.Background()
	_, err := plugin.GetSnapshot(ctx, "nonexistent")

	if err == nil {
		t.Error("GetSnapshot should error for nonexistent camera")
	}
}

func TestPlugin_ParseConfig_WithDevices(t *testing.T) {
	plugin := NewPlugin()

	config := map[string]interface{}{
		"devices": []interface{}{
			map[string]interface{}{
				"host":     "192.168.1.100",
				"port":     float64(80),
				"username": "admin",
				"password": "password",
				"name":     "Front Camera",
			},
			map[string]interface{}{
				"host":     "192.168.1.101",
				"username": "admin",
				"password": "secret",
			},
		},
	}

	err := plugin.parseConfig(config)

	if err != nil {
		t.Errorf("parseConfig should not error: %v", err)
	}
	if len(plugin.devices) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(plugin.devices))
	}

	d1 := plugin.devices[0]
	if d1.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", d1.Host)
	}
	if d1.Port != 80 {
		t.Errorf("Expected port 80, got %d", d1.Port)
	}
	if d1.Name != "Front Camera" {
		t.Errorf("Expected name 'Front Camera', got '%s'", d1.Name)
	}
}

func TestPlugin_ParseConfig_EmptyHost(t *testing.T) {
	plugin := NewPlugin()

	config := map[string]interface{}{
		"devices": []interface{}{
			map[string]interface{}{
				"host":     "", // Empty host should be skipped
				"username": "admin",
				"password": "password",
			},
		},
	}

	err := plugin.parseConfig(config)

	if err != nil {
		t.Errorf("parseConfig should not error: %v", err)
	}
	if len(plugin.devices) != 0 {
		t.Errorf("Expected 0 devices (empty host skipped), got %d", len(plugin.devices))
	}
}

func TestPlugin_HandleRequest_Initialize(t *testing.T) {
	plugin := NewPlugin()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  nil,
	}

	resp := plugin.HandleRequest(req)

	if resp.Error != nil {
		t.Errorf("Initialize should not return error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("Initialize should return result")
	}
}

func TestPlugin_HandleRequest_Shutdown(t *testing.T) {
	plugin := NewPlugin()
	ctx := context.Background()
	_ = plugin.Initialize(ctx, nil)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "shutdown",
	}

	resp := plugin.HandleRequest(req)

	if resp.Error != nil {
		t.Errorf("Shutdown should not return error: %v", resp.Error)
	}
}

func TestPlugin_HandleRequest_Health(t *testing.T) {
	plugin := NewPlugin()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "health",
	}

	resp := plugin.HandleRequest(req)

	if resp.Error != nil {
		t.Errorf("Health should not return error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("Health should return result")
	}
}

func TestPlugin_HandleRequest_ListCameras(t *testing.T) {
	plugin := NewPlugin()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "list_cameras",
	}

	resp := plugin.HandleRequest(req)

	if resp.Error != nil {
		t.Errorf("ListCameras should not return error: %v", resp.Error)
	}
}

func TestPlugin_HandleRequest_DiscoverCameras(t *testing.T) {
	plugin := NewPlugin()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "discover_cameras",
	}

	resp := plugin.HandleRequest(req)

	if resp.Error != nil {
		t.Errorf("DiscoverCameras should not return error: %v", resp.Error)
	}
}

func TestPlugin_HandleRequest_GetCamera_NotFound(t *testing.T) {
	plugin := NewPlugin()

	params, _ := json.Marshal(map[string]string{"camera_id": "nonexistent"})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "get_camera",
		Params:  params,
	}

	resp := plugin.HandleRequest(req)

	if resp.Error == nil {
		t.Error("GetCamera should return error for nonexistent camera")
	}
}

func TestPlugin_HandleRequest_RemoveCamera_NotFound(t *testing.T) {
	plugin := NewPlugin()

	params, _ := json.Marshal(map[string]string{"camera_id": "nonexistent"})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "remove_camera",
		Params:  params,
	}

	resp := plugin.HandleRequest(req)

	if resp.Error == nil {
		t.Error("RemoveCamera should return error for nonexistent camera")
	}
}

func TestPlugin_HandleRequest_UnknownMethod(t *testing.T) {
	plugin := NewPlugin()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "unknown_method",
	}

	resp := plugin.HandleRequest(req)

	if resp.Error == nil {
		t.Error("Unknown method should return error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("Expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestPlugin_HandleRequest_InvalidParams(t *testing.T) {
	plugin := NewPlugin()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "add_camera",
		Params:  []byte("invalid json{"),
	}

	resp := plugin.HandleRequest(req)

	if resp.Error == nil {
		t.Error("Invalid params should return error")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("Expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestPlugin_HandleRequest_PTZControl_NotFound(t *testing.T) {
	plugin := NewPlugin()

	params, _ := json.Marshal(map[string]interface{}{
		"camera_id": "nonexistent",
		"command":   PTZCommand{Action: "pan", Direction: 1},
	})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "ptz_control",
		Params:  params,
	}

	resp := plugin.HandleRequest(req)

	if resp.Error == nil {
		t.Error("PTZControl should return error for nonexistent camera")
	}
}

func TestPlugin_HandleRequest_GetSnapshot_NotFound(t *testing.T) {
	plugin := NewPlugin()

	params, _ := json.Marshal(map[string]string{"camera_id": "nonexistent"})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "get_snapshot",
		Params:  params,
	}

	resp := plugin.HandleRequest(req)

	if resp.Error == nil {
		t.Error("GetSnapshot should return error for nonexistent camera")
	}
}

// JSON-RPC Types tests

func TestJSONRPCRequest(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
		Params:  []byte(`{"key": "value"}`),
	}

	if req.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got '%s'", req.JSONRPC)
	}
	if req.Method != "test" {
		t.Errorf("Expected method 'test', got '%s'", req.Method)
	}
}

func TestJSONRPCResponse(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  map[string]interface{}{"status": "ok"},
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got '%s'", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Error("Error should be nil")
	}
}

func TestJSONRPCError(t *testing.T) {
	err := JSONRPCError{
		Code:    -32600,
		Message: "Invalid Request",
		Data:    "additional info",
	}

	if err.Code != -32600 {
		t.Errorf("Expected code -32600, got %d", err.Code)
	}
	if err.Message != "Invalid Request" {
		t.Errorf("Expected message 'Invalid Request', got '%s'", err.Message)
	}
}

func TestDeviceConfig(t *testing.T) {
	config := DeviceConfig{
		Host:     "192.168.1.100",
		Port:     80,
		Username: "admin",
		Password: "password",
		Channels: []int{0, 1, 2},
		Name:     "NVR",
	}

	if config.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", config.Host)
	}
	if len(config.Channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(config.Channels))
	}
}

func TestCameraConfig(t *testing.T) {
	config := CameraConfig{
		Host:     "192.168.1.100",
		Port:     80,
		Username: "admin",
		Password: "password",
		Channel:  0,
		Name:     "Front Door",
		Extra:    map[string]interface{}{"custom": "value"},
	}

	if config.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", config.Host)
	}
	if config.Extra["custom"] != "value" {
		t.Errorf("Expected extra 'value', got %v", config.Extra["custom"])
	}
}

func TestPluginCamera(t *testing.T) {
	cam := PluginCamera{
		ID:           "cam_1",
		PluginID:     "reolink",
		Name:         "Front Door",
		Model:        "RLC-810A",
		Host:         "192.168.1.100",
		MainStream:   "rtsp://...",
		SubStream:    "rtsp://...",
		SnapshotURL:  "http://...",
		Capabilities: []string{"video", "ptz"},
		Online:       true,
		LastSeen:     time.Now().Format(time.RFC3339),
	}

	if cam.ID != "cam_1" {
		t.Errorf("Expected ID 'cam_1', got '%s'", cam.ID)
	}
	if len(cam.Capabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(cam.Capabilities))
	}
}

func TestDiscoveredCamera(t *testing.T) {
	cam := DiscoveredCamera{
		ID:              "cam_1",
		Name:            "Front Door",
		Model:           "RLC-810A",
		Manufacturer:    "Reolink",
		Host:            "192.168.1.100",
		Port:            80,
		Channels:        1,
		Capabilities:    []string{"video"},
		FirmwareVersion: "v3.0.0",
		Serial:          "ABC123",
	}

	if cam.Manufacturer != "Reolink" {
		t.Errorf("Expected manufacturer 'Reolink', got '%s'", cam.Manufacturer)
	}
}

func TestHealthStatus(t *testing.T) {
	status := HealthStatus{
		State:     "healthy",
		Message:   "All cameras online",
		LastCheck: time.Now().Format(time.RFC3339),
		Details:   map[string]interface{}{"cameras_online": 5},
	}

	if status.State != "healthy" {
		t.Errorf("Expected state 'healthy', got '%s'", status.State)
	}
}
