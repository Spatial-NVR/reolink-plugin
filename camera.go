package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// Camera represents a Reolink camera instance
type Camera struct {
	id       string
	name     string
	model    string
	host     string
	channel  int
	protocol string // "rtsp" (default), "hls", or "rtmp"
	client   *Client

	ability   *Ability
	encConfig *EncoderConfig

	online   bool
	lastSeen time.Time

	mu sync.RWMutex
}

// NewCamera creates a new Reolink camera instance
func NewCamera(id, name, model, host string, channel int, client *Client) *Camera {
	return &Camera{
		id:       id,
		name:     name,
		model:    model,
		host:     host,
		channel:  channel,
		protocol: "rtsp", // Default to RTSP for better audio support
		client:   client,
		online:   true,
		lastSeen: time.Now(),
	}
}

// SetProtocol sets the streaming protocol for this camera
func (c *Camera) SetProtocol(protocol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if protocol == "" {
		protocol = "rtsp"
	}
	c.protocol = protocol
}

// Protocol returns the current streaming protocol
func (c *Camera) Protocol() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.protocol == "" {
		return "rtsp"
	}
	return c.protocol
}

func (c *Camera) ID() string      { return c.id }
func (c *Camera) Name() string    { return c.name }
func (c *Camera) Model() string   { return c.model }
func (c *Camera) Host() string    { return c.host }
func (c *Camera) Channel() int    { return c.channel }

// DeviceType returns the type of device (camera, doorbell, nvr, battery)
func (c *Camera) DeviceType() string {
	if isDoorbellModel(c.model) {
		return "doorbell"
	}
	if isBatteryModel(c.model) {
		return "battery"
	}
	// Check if it's an NVR based on channel count
	if c.client != nil {
		info := c.client.GetCachedDeviceInfo()
		if info != nil && info.ChannelCount > 1 {
			return "nvr"
		}
	}
	return "camera"
}

func (c *Camera) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

func (c *Camera) LastSeen() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSeen
}

func (c *Camera) SetAbility(ability *Ability) {
	c.mu.Lock()
	c.ability = ability
	c.mu.Unlock()
}

func (c *Camera) SetEncoderConfig(cfg *EncoderConfig) {
	c.mu.Lock()
	c.encConfig = cfg
	c.mu.Unlock()
}

func (c *Camera) Capabilities() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	caps := []string{"video", "snapshot"}

	if c.ability != nil {
		if c.ability.PTZ || c.ability.PanTilt {
			caps = append(caps, "ptz")
		}
		if c.ability.TwoWayAudio {
			caps = append(caps, "two_way_audio")
		}
		if c.ability.AudioAlarm {
			caps = append(caps, "audio")
		}
	}

	// Detect from model
	model := c.model
	if isDoorbellModel(model) {
		caps = append(caps, "doorbell")
	}
	if isBatteryModel(model) {
		caps = append(caps, "battery")
	}
	if hasAIDetection(model) {
		caps = append(caps, "ai_detection", "motion")
	}

	return caps
}

func (c *Camera) StreamURL(quality string) string {
	protocol := c.Protocol()
	if quality == "main" {
		return c.client.StreamURL(c.channel, "main", protocol)
	}
	return c.client.StreamURL(c.channel, "sub", protocol)
}

func (c *Camera) SnapshotURL() string {
	return fmt.Sprintf("http://%s:%d/cgi-bin/api.cgi?cmd=Snap&channel=%d",
		c.host, c.client.port, c.channel)
}

func (c *Camera) PTZControl(ctx context.Context, cmd PTZCommand) error {
	ptzCmd := PTZCmd{Speed: 30}

	switch cmd.Action {
	case "pan":
		if cmd.Direction < 0 {
			ptzCmd.Operation = "Left"
		} else {
			ptzCmd.Operation = "Right"
		}
	case "tilt":
		if cmd.Direction < 0 {
			ptzCmd.Operation = "Down"
		} else {
			ptzCmd.Operation = "Up"
		}
	case "zoom":
		if cmd.Direction < 0 {
			ptzCmd.Operation = "ZoomDec"
		} else {
			ptzCmd.Operation = "ZoomInc"
		}
	case "stop":
		ptzCmd.Operation = "Stop"
	case "preset":
		ptzCmd.Operation = "ToPos"
		ptzCmd.Preset = cmd.Preset
	default:
		return fmt.Errorf("unknown PTZ action: %s", cmd.Action)
	}

	if cmd.Speed > 0 {
		ptzCmd.Speed = int(cmd.Speed * 64)
	}

	return c.client.PTZControl(ctx, c.channel, ptzCmd)
}

func (c *Camera) GetSnapshot(ctx context.Context) (string, error) {
	data, err := c.client.GetSnapshot(ctx, c.channel)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// StreamURLForProtocol returns the stream URL for a specific protocol
func (c *Camera) StreamURLForProtocol(quality, protocol string) string {
	return c.client.StreamURL(c.channel, quality, protocol)
}

// CameraPreset represents a PTZ preset from the camera
type CameraPreset struct {
	ID   string
	Name string
}

// GetPTZPresets returns the available PTZ presets for this camera
func (c *Camera) GetPTZPresets(ctx context.Context) ([]CameraPreset, error) {
	presets, err := c.client.GetPTZPresets(ctx, c.channel)
	if err != nil {
		return nil, err
	}

	var result []CameraPreset
	for _, p := range presets {
		result = append(result, CameraPreset{
			ID:   fmt.Sprintf("%d", p.ID), // Convert int to string
			Name: p.Name,
		})
	}

	return result, nil
}

// CameraDeviceInfo represents device information
type CameraDeviceInfo struct {
	Model           string
	Serial          string
	FirmwareVersion string
	HardwareVersion string
	ChannelCount    int
}

// GetDeviceInfo returns the device information for this camera
func (c *Camera) GetDeviceInfo() *CameraDeviceInfo {
	if c.client == nil {
		return nil
	}

	info := c.client.GetCachedDeviceInfo()
	if info == nil {
		return nil
	}

	return &CameraDeviceInfo{
		Model:           info.Model,
		Serial:          info.Serial,
		FirmwareVersion: info.FirmwareVersion,
		HardwareVersion: info.HardwareVersion,
		ChannelCount:    info.ChannelCount,
	}
}

// Helper functions for model detection
func isDoorbellModel(model string) bool {
	return containsIgnoreCase(model, "doorbell")
}

func isBatteryModel(model string) bool {
	keywords := []string{"argus", "lumus", "go", "battery"}
	for _, kw := range keywords {
		if containsIgnoreCase(model, kw) {
			return true
		}
	}
	return false
}

func hasAIDetection(model string) bool {
	noAI := []string{"rlc-410", "rlc-420", "e1 zoom", "c1 pro"}
	for _, m := range noAI {
		if containsIgnoreCase(model, m) {
			return false
		}
	}
	return true
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 &&
		(s[0]|0x20 == substr[0]|0x20) && containsIgnoreCase(s[1:], substr[1:]) ||
		len(s) > 0 && containsIgnoreCase(s[1:], substr))
}
