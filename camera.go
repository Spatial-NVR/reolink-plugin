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
	id      string
	name    string
	model   string
	host    string
	channel int
	client  *Client

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
		client:   client,
		online:   true,
		lastSeen: time.Now(),
	}
}

func (c *Camera) ID() string      { return c.id }
func (c *Camera) Name() string    { return c.name }
func (c *Camera) Model() string   { return c.model }
func (c *Camera) Host() string    { return c.host }
func (c *Camera) Channel() int    { return c.channel }

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
	if quality == "main" {
		return c.client.RTSPStreamURL(c.channel, "main")
	}
	return c.client.RTSPStreamURL(c.channel, "sub")
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
