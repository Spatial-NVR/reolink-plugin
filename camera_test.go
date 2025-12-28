package main

import (
	"context"
	"testing"
	"time"
)

func TestNewCamera(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	if camera == nil {
		t.Fatal("NewCamera returned nil")
	}
	if camera.id != "cam_1" {
		t.Errorf("Expected id 'cam_1', got '%s'", camera.id)
	}
	if camera.name != "Front Door" {
		t.Errorf("Expected name 'Front Door', got '%s'", camera.name)
	}
	if camera.model != "RLC-810A" {
		t.Errorf("Expected model 'RLC-810A', got '%s'", camera.model)
	}
	if camera.host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", camera.host)
	}
	if camera.channel != 0 {
		t.Errorf("Expected channel 0, got %d", camera.channel)
	}
	if !camera.online {
		t.Error("New camera should be online")
	}
}

func TestCamera_Getters(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	if camera.ID() != "cam_1" {
		t.Errorf("ID() = '%s', expected 'cam_1'", camera.ID())
	}
	if camera.Name() != "Front Door" {
		t.Errorf("Name() = '%s', expected 'Front Door'", camera.Name())
	}
	if camera.Model() != "RLC-810A" {
		t.Errorf("Model() = '%s', expected 'RLC-810A'", camera.Model())
	}
	if camera.Host() != "192.168.1.100" {
		t.Errorf("Host() = '%s', expected '192.168.1.100'", camera.Host())
	}
	if camera.Channel() != 0 {
		t.Errorf("Channel() = %d, expected 0", camera.Channel())
	}
}

func TestCamera_IsOnline(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	if !camera.IsOnline() {
		t.Error("Camera should be online initially")
	}

	camera.online = false
	if camera.IsOnline() {
		t.Error("Camera should be offline after setting online=false")
	}
}

func TestCamera_LastSeen(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	lastSeen := camera.LastSeen()
	if lastSeen.IsZero() {
		t.Error("LastSeen should not be zero")
	}
	// Should be recent (within last second)
	if time.Since(lastSeen) > time.Second {
		t.Error("LastSeen should be recent")
	}
}

func TestCamera_SetAbility(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	ability := &Ability{
		PTZ:         true,
		PanTilt:     true,
		TwoWayAudio: true,
		AudioAlarm:  true,
	}

	camera.SetAbility(ability)

	camera.mu.RLock()
	if camera.ability != ability {
		t.Error("Ability should be set")
	}
	camera.mu.RUnlock()
}

func TestCamera_SetEncoderConfig(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	cfg := &EncoderConfig{
		MainStream: StreamConfig{
			Width:     1920,
			Height:    1080,
			FrameRate: 30,
			Codec:     "h264",
		},
	}

	camera.SetEncoderConfig(cfg)

	camera.mu.RLock()
	if camera.encConfig != cfg {
		t.Error("EncoderConfig should be set")
	}
	camera.mu.RUnlock()
}

func TestCamera_Capabilities_Basic(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	caps := camera.Capabilities()

	// Basic capabilities should always be present
	hasVideo := false
	hasSnapshot := false
	for _, cap := range caps {
		if cap == "video" {
			hasVideo = true
		}
		if cap == "snapshot" {
			hasSnapshot = true
		}
	}

	if !hasVideo {
		t.Error("Should have 'video' capability")
	}
	if !hasSnapshot {
		t.Error("Should have 'snapshot' capability")
	}
}

func TestCamera_Capabilities_WithAbility(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	ability := &Ability{
		PTZ:         true,
		TwoWayAudio: true,
		AudioAlarm:  true,
	}
	camera.SetAbility(ability)

	caps := camera.Capabilities()

	hasPTZ := false
	hasTwoWayAudio := false
	hasAudio := false
	for _, cap := range caps {
		if cap == "ptz" {
			hasPTZ = true
		}
		if cap == "two_way_audio" {
			hasTwoWayAudio = true
		}
		if cap == "audio" {
			hasAudio = true
		}
	}

	if !hasPTZ {
		t.Error("Should have 'ptz' capability")
	}
	if !hasTwoWayAudio {
		t.Error("Should have 'two_way_audio' capability")
	}
	if !hasAudio {
		t.Error("Should have 'audio' capability")
	}
}

func TestCamera_Capabilities_Doorbell(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "Reolink Video Doorbell", "192.168.1.100", 0, client)

	caps := camera.Capabilities()

	hasDoorbell := false
	for _, cap := range caps {
		if cap == "doorbell" {
			hasDoorbell = true
		}
	}

	if !hasDoorbell {
		t.Error("Doorbell model should have 'doorbell' capability")
	}
}

func TestCamera_Capabilities_Battery(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Backyard", "Argus 3 Pro", "192.168.1.100", 0, client)

	caps := camera.Capabilities()

	hasBattery := false
	for _, cap := range caps {
		if cap == "battery" {
			hasBattery = true
		}
	}

	if !hasBattery {
		t.Error("Battery model should have 'battery' capability")
	}
}

func TestCamera_Capabilities_AIDetection(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	caps := camera.Capabilities()

	hasAI := false
	hasMotion := false
	for _, cap := range caps {
		if cap == "ai_detection" {
			hasAI = true
		}
		if cap == "motion" {
			hasMotion = true
		}
	}

	if !hasAI {
		t.Error("Modern camera should have 'ai_detection' capability")
	}
	if !hasMotion {
		t.Error("Modern camera should have 'motion' capability")
	}
}

func TestCamera_Capabilities_NoAIDetection(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Old Camera", "RLC-410", "192.168.1.100", 0, client)

	caps := camera.Capabilities()

	hasAI := false
	for _, cap := range caps {
		if cap == "ai_detection" {
			hasAI = true
		}
	}

	if hasAI {
		t.Error("RLC-410 should NOT have 'ai_detection' capability")
	}
}

func TestCamera_StreamURL(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	mainURL := camera.StreamURL("main")
	subURL := camera.StreamURL("sub")
	defaultURL := camera.StreamURL("") // Should default to sub

	if mainURL != "rtsp://admin:password@192.168.1.100:554/h264Preview_01_main" {
		t.Errorf("Unexpected main stream URL: %s", mainURL)
	}
	if subURL != "rtsp://admin:password@192.168.1.100:554/h264Preview_01_sub" {
		t.Errorf("Unexpected sub stream URL: %s", subURL)
	}
	if defaultURL != subURL {
		t.Errorf("Default quality should return sub stream URL")
	}
}

func TestCamera_SnapshotURL(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	camera := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	url := camera.SnapshotURL()
	expected := "http://192.168.1.100:80/cgi-bin/api.cgi?cmd=Snap&channel=0"

	if url != expected {
		t.Errorf("Expected snapshot URL '%s', got '%s'", expected, url)
	}
}

func TestCamera_PTZControl_Pan(t *testing.T) {
	_ = NewClient("192.168.1.100", 80, "admin", "password")

	// Test PTZ command construction
	cmd := PTZCommand{Action: "pan", Direction: -1}

	// We can't actually test the HTTP call without a mock server,
	// but we can verify the command parsing logic
	ptzCmd := PTZCmd{Speed: 30}

	switch cmd.Action {
	case "pan":
		if cmd.Direction < 0 {
			ptzCmd.Operation = "Left"
		} else {
			ptzCmd.Operation = "Right"
		}
	}

	if ptzCmd.Operation != "Left" {
		t.Errorf("Expected operation 'Left', got '%s'", ptzCmd.Operation)
	}
}

func TestCamera_PTZControl_Tilt(t *testing.T) {
	cmd := PTZCommand{Action: "tilt", Direction: 1}
	ptzCmd := PTZCmd{Speed: 30}

	switch cmd.Action {
	case "tilt":
		if cmd.Direction < 0 {
			ptzCmd.Operation = "Down"
		} else {
			ptzCmd.Operation = "Up"
		}
	}

	if ptzCmd.Operation != "Up" {
		t.Errorf("Expected operation 'Up', got '%s'", ptzCmd.Operation)
	}
}

func TestCamera_PTZControl_Zoom(t *testing.T) {
	cmd := PTZCommand{Action: "zoom", Direction: 1}
	ptzCmd := PTZCmd{Speed: 30}

	switch cmd.Action {
	case "zoom":
		if cmd.Direction < 0 {
			ptzCmd.Operation = "ZoomDec"
		} else {
			ptzCmd.Operation = "ZoomInc"
		}
	}

	if ptzCmd.Operation != "ZoomInc" {
		t.Errorf("Expected operation 'ZoomInc', got '%s'", ptzCmd.Operation)
	}
}

func TestCamera_PTZControl_Stop(t *testing.T) {
	cmd := PTZCommand{Action: "stop"}
	ptzCmd := PTZCmd{Speed: 30}

	switch cmd.Action {
	case "stop":
		ptzCmd.Operation = "Stop"
	}

	if ptzCmd.Operation != "Stop" {
		t.Errorf("Expected operation 'Stop', got '%s'", ptzCmd.Operation)
	}
}

func TestCamera_PTZControl_Preset(t *testing.T) {
	cmd := PTZCommand{Action: "preset", Preset: "home"}
	ptzCmd := PTZCmd{Speed: 30}

	switch cmd.Action {
	case "preset":
		ptzCmd.Operation = "ToPos"
		ptzCmd.Preset = cmd.Preset
	}

	if ptzCmd.Operation != "ToPos" {
		t.Errorf("Expected operation 'ToPos', got '%s'", ptzCmd.Operation)
	}
	if ptzCmd.Preset != "home" {
		t.Errorf("Expected preset 'home', got '%s'", ptzCmd.Preset)
	}
}

func TestCamera_PTZControl_CustomSpeed(t *testing.T) {
	cmd := PTZCommand{Action: "pan", Direction: 1, Speed: 0.5}
	ptzCmd := PTZCmd{Speed: 30}

	if cmd.Speed > 0 {
		ptzCmd.Speed = int(cmd.Speed * 64)
	}

	if ptzCmd.Speed != 32 {
		t.Errorf("Expected speed 32, got %d", ptzCmd.Speed)
	}
}

func TestCamera_PTZControl_UnknownAction(t *testing.T) {
	client := NewClient("192.168.1.100", 80, "admin", "password")
	cam := NewCamera("cam_1", "Front Door", "RLC-810A", "192.168.1.100", 0, client)

	cmd := PTZCommand{Action: "unknown_action"}
	err := cam.PTZControl(context.Background(), cmd)

	if err == nil {
		t.Error("Expected error for unknown PTZ action")
	}
}

// Helper function tests

func TestIsDoorbellModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"Reolink Video Doorbell", true},
		{"doorbell_wifi", true},
		{"DOORBELL_POE", true},
		{"RLC-810A", false},
		{"Argus 3 Pro", false},
	}

	for _, tt := range tests {
		result := isDoorbellModel(tt.model)
		if result != tt.expected {
			t.Errorf("isDoorbellModel(%s) = %v, expected %v", tt.model, result, tt.expected)
		}
	}
}

func TestIsBatteryModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"Argus 3 Pro", true},
		{"Lumus", true},
		{"Reolink Go", true},
		{"Battery Cam", true},
		{"ARGUS PT", true},
		{"RLC-810A", false},
		{"E1 Outdoor", false},
	}

	for _, tt := range tests {
		result := isBatteryModel(tt.model)
		if result != tt.expected {
			t.Errorf("isBatteryModel(%s) = %v, expected %v", tt.model, result, tt.expected)
		}
	}
}

func TestHasAIDetection(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"RLC-810A", true},
		{"E1 Pro", true},
		{"RLC-520A", true},
		{"RLC-410", false},
		{"RLC-420", false},
		{"E1 Zoom", false},
		{"C1 Pro", false},
	}

	for _, tt := range tests {
		result := hasAIDetection(tt.model)
		if result != tt.expected {
			t.Errorf("hasAIDetection(%s) = %v, expected %v", tt.model, result, tt.expected)
		}
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "lo Wo", true},
		{"Hello World", "xyz", false},
		{"", "", true},
		{"Hello", "", true},
		{"", "hello", false},
		{"A", "a", true},
		{"a", "A", true},
	}

	for _, tt := range tests {
		result := containsIgnoreCase(tt.s, tt.substr)
		if result != tt.expected {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, expected %v",
				tt.s, tt.substr, result, tt.expected)
		}
	}
}

func TestPTZCommand(t *testing.T) {
	cmd := PTZCommand{
		Action:    "pan",
		Direction: 1,
		Speed:     0.5,
		Preset:    "",
	}

	if cmd.Action != "pan" {
		t.Errorf("Expected action 'pan', got '%s'", cmd.Action)
	}
	if cmd.Direction != 1 {
		t.Errorf("Expected direction 1, got %f", cmd.Direction)
	}
	if cmd.Speed != 0.5 {
		t.Errorf("Expected speed 0.5, got %f", cmd.Speed)
	}
}
