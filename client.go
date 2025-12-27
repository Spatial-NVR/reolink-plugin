package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is an HTTP client for the Reolink API
type Client struct {
	host     string
	port     int
	username string
	password string

	token        string
	tokenExp     time.Time
	useBasicAuth bool // If true, use URL-based auth instead of token

	http *http.Client
	mu   sync.RWMutex
}

// NewClient creates a new Reolink API client
func NewClient(host string, port int, username, password string) *Client {
	if port == 0 {
		port = 80
	}
	// Create HTTP client that accepts self-signed certs for HTTPS
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &Client{
		host:     host,
		port:     port,
		username: username,
		password: password,
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
		},
	}
}

func (c *Client) baseURL() string {
	// Use HTTPS for port 443, otherwise HTTP
	if c.port == 443 {
		return fmt.Sprintf("https://%s", c.host)
	}
	return fmt.Sprintf("http://%s:%d", c.host, c.port)
}

func (c *Client) baseURLHTTPS() string {
	if c.port == 443 || c.port == 80 {
		return fmt.Sprintf("https://%s", c.host)
	}
	return fmt.Sprintf("https://%s:%d", c.host, c.port)
}

func (c *Client) apiURL() string {
	return c.baseURL() + "/api.cgi"
}

// Login authenticates and obtains a session token
// Uses two-stage approach like Scrypted: try basic auth first, then token-based login
func (c *Client) Login(ctx context.Context) error {
	// First, try basic auth by testing GetDevInfo with credentials in URL
	// This works on some older firmware and avoids token management
	log.Printf("Attempting login to %s:%d as user '%s'", c.host, c.port, c.username)

	if err := c.tryBasicAuth(ctx); err == nil {
		log.Printf("Basic auth succeeded for %s", c.host)
		return nil
	} else {
		log.Printf("Basic auth failed for %s: %v, trying token-based login", c.host, err)
	}

	// Fall back to token-based Login API
	cmd := []apiCommand{{
		Cmd:    "Login",
		Action: 0,
		Param: map[string]interface{}{
			"User": map[string]interface{}{
				"userName": c.username,
				"password": c.password,
			},
		},
	}}

	// Try HTTP first
	log.Printf("Trying HTTP login to %s", c.baseURL()+"/api.cgi")
	resp, err := c.doRequestURL(ctx, c.baseURL()+"/api.cgi", cmd)
	if err != nil {
		// If HTTP fails, try HTTPS
		log.Printf("HTTP login failed for %s, trying HTTPS: %v", c.host, err)
		resp, err = c.doRequestURL(ctx, c.baseURLHTTPS()+"/api.cgi", cmd)
		if err != nil {
			return fmt.Errorf("login request failed (tried HTTP and HTTPS): %w", err)
		}
	}

	if len(resp) == 0 {
		return fmt.Errorf("empty login response")
	}

	loginResp := resp[0]
	log.Printf("Login response for %s: cmd=%s code=%d", c.host, loginResp.Cmd, loginResp.Code)

	if loginResp.Code != 0 {
		return fmt.Errorf("login failed: %s", reolinkErrorMessage(loginResp.Code))
	}

	value, ok := loginResp.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid login response format")
	}

	token, ok := value["Token"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing token in response")
	}

	tokenName, ok := token["name"].(string)
	if !ok {
		return fmt.Errorf("missing token name")
	}

	leaseTime := 3600
	if lt, ok := token["leaseTime"].(float64); ok {
		leaseTime = int(lt)
	}

	c.mu.Lock()
	c.token = tokenName
	c.tokenExp = time.Now().Add(time.Duration(leaseTime-60) * time.Second)
	c.mu.Unlock()

	log.Printf("Token-based login succeeded for %s, token expires in %d seconds", c.host, leaseTime)
	return nil
}

// tryBasicAuth attempts to access the API with credentials in the URL (like older firmware)
func (c *Client) tryBasicAuth(ctx context.Context) error {
	// Try with credentials in URL query string
	authURL := fmt.Sprintf("%s/api.cgi?cmd=GetDevInfo&user=%s&password=%s",
		c.baseURL(), url.QueryEscape(c.username), url.QueryEscape(c.password))

	req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Try HTTPS
		authURL = fmt.Sprintf("%s/api.cgi?cmd=GetDevInfo&user=%s&password=%s",
			c.baseURLHTTPS(), url.QueryEscape(c.username), url.QueryEscape(c.password))
		req, err = http.NewRequestWithContext(ctx, "GET", authURL, nil)
		if err != nil {
			return err
		}
		resp, err = c.http.Do(req)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var responses []apiResponse
	if err := json.Unmarshal(body, &responses); err != nil {
		return err
	}

	if len(responses) == 0 || responses[0].Code != 0 {
		if len(responses) > 0 {
			return fmt.Errorf("code %d", responses[0].Code)
		}
		return fmt.Errorf("empty response")
	}

	// Basic auth worked, set a flag to use URL-based auth instead of token
	c.mu.Lock()
	c.useBasicAuth = true
	c.mu.Unlock()

	return nil
}

func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	useBasic := c.useBasicAuth
	needLogin := !useBasic && (c.token == "" || time.Now().After(c.tokenExp))
	c.mu.RUnlock()

	if needLogin {
		return c.Login(ctx)
	}
	return nil
}

// GetDeviceInfo retrieves basic device information
func (c *Client) GetDeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	cmd := []apiCommand{{
		Cmd:    "GetDevInfo",
		Action: 0,
		Param:  map[string]interface{}{},
	}}

	resp, err := c.doRequest(ctx, cmd, true)
	if err != nil {
		return nil, err
	}

	if len(resp) == 0 || resp[0].Code != 0 {
		return nil, fmt.Errorf("GetDevInfo failed")
	}

	value, ok := resp[0].Value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid device info format")
	}

	devInfo, ok := value["DevInfo"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing DevInfo")
	}

	info := &DeviceInfo{}
	if v, ok := devInfo["model"].(string); ok {
		info.Model = v
	}
	if v, ok := devInfo["name"].(string); ok {
		info.Name = v
	}
	if v, ok := devInfo["serial"].(string); ok {
		info.Serial = v
	}
	if v, ok := devInfo["firmVer"].(string); ok {
		info.FirmwareVersion = v
	}
	if v, ok := devInfo["channelNum"].(float64); ok {
		info.ChannelCount = int(v)
	}
	if info.ChannelCount == 0 {
		info.ChannelCount = 1
	}

	return info, nil
}

// GetAbility retrieves camera capabilities
func (c *Client) GetAbility(ctx context.Context, channel int) (*Ability, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	cmd := []apiCommand{{
		Cmd:    "GetAbility",
		Action: 0,
		Param: map[string]interface{}{
			"User": map[string]interface{}{
				"userName": c.username,
			},
		},
	}}

	resp, err := c.doRequest(ctx, cmd, true)
	if err != nil {
		return nil, err
	}

	if len(resp) == 0 || resp[0].Code != 0 {
		return nil, fmt.Errorf("GetAbility failed")
	}

	ability := &Ability{}
	value, ok := resp[0].Value.(map[string]interface{})
	if !ok {
		return ability, nil
	}

	abilityData, ok := value["Ability"].(map[string]interface{})
	if !ok {
		return ability, nil
	}

	if ptz, ok := abilityData["ptz"].(map[string]interface{}); ok {
		if ver, ok := ptz["ver"].(float64); ok && ver > 0 {
			ability.PTZ = true
		}
	}
	if pt, ok := abilityData["pt"].(map[string]interface{}); ok {
		if ver, ok := pt["ver"].(float64); ok && ver > 0 {
			ability.PanTilt = true
		}
	}
	if audio, ok := abilityData["supportAudioAlarm"].(map[string]interface{}); ok {
		if ver, ok := audio["ver"].(float64); ok && ver > 0 {
			ability.AudioAlarm = true
		}
	}
	if talk, ok := abilityData["talk"].(map[string]interface{}); ok {
		if ver, ok := talk["ver"].(float64); ok && ver > 0 {
			ability.TwoWayAudio = true
		}
	}

	return ability, nil
}

// GetEncoderConfig retrieves video encoder settings
func (c *Client) GetEncoderConfig(ctx context.Context, channel int) (*EncoderConfig, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	cmd := []apiCommand{{
		Cmd:    "GetEnc",
		Action: 0,
		Param: map[string]interface{}{
			"channel": channel,
		},
	}}

	resp, err := c.doRequest(ctx, cmd, true)
	if err != nil {
		return nil, err
	}

	if len(resp) == 0 || resp[0].Code != 0 {
		return nil, fmt.Errorf("GetEnc failed")
	}

	cfg := &EncoderConfig{}
	value, ok := resp[0].Value.(map[string]interface{})
	if !ok {
		return cfg, nil
	}

	if enc, ok := value["Enc"].(map[string]interface{}); ok {
		if main, ok := enc["mainStream"].(map[string]interface{}); ok {
			cfg.MainStream = parseStreamConfig(main)
		}
		if sub, ok := enc["subStream"].(map[string]interface{}); ok {
			cfg.SubStream = parseStreamConfig(sub)
		}
	}

	return cfg, nil
}

func parseStreamConfig(data map[string]interface{}) StreamConfig {
	cfg := StreamConfig{}
	if v, ok := data["width"].(float64); ok {
		cfg.Width = int(v)
	}
	if v, ok := data["height"].(float64); ok {
		cfg.Height = int(v)
	}
	if v, ok := data["frameRate"].(float64); ok {
		cfg.FrameRate = int(v)
	}
	if v, ok := data["bitRate"].(float64); ok {
		cfg.BitRate = int(v)
	}
	if v, ok := data["video"].(map[string]interface{}); ok {
		if codec, ok := v["videoType"].(string); ok {
			cfg.Codec = codec
		}
	}
	return cfg
}

// PTZControl sends a PTZ command
func (c *Client) PTZControl(ctx context.Context, channel int, cmd PTZCmd) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	apiCmd := []apiCommand{{
		Cmd:    "PtzCtrl",
		Action: 0,
		Param: map[string]interface{}{
			"channel": channel,
			"op":      cmd.Operation,
			"speed":   cmd.Speed,
		},
	}}

	if cmd.Preset != "" {
		apiCmd[0].Param["id"] = cmd.Preset
	}

	resp, err := c.doRequest(ctx, apiCmd, true)
	if err != nil {
		return err
	}

	if len(resp) > 0 && resp[0].Code != 0 {
		return fmt.Errorf("PTZ command failed: code %d", resp[0].Code)
	}

	return nil
}

// GetSnapshot captures a JPEG snapshot
func (c *Client) GetSnapshot(ctx context.Context, channel int) ([]byte, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	snapURL := fmt.Sprintf("%s/cgi-bin/api.cgi?cmd=Snap&channel=%d&token=%s",
		c.baseURL(), channel, token)

	req, err := http.NewRequestWithContext(ctx, "GET", snapURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot failed: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// ProbeCamera fully probes a camera and returns all detected information
func (c *Client) ProbeCamera(ctx context.Context) (*CameraProbeResult, error) {
	result := &CameraProbeResult{
		Host:     c.host,
		Port:     c.port,
		Channels: []ChannelInfo{},
	}

	devInfo, err := c.GetDeviceInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device info: %w", err)
	}

	result.Model = devInfo.Model
	result.Name = devInfo.Name
	result.Serial = devInfo.Serial
	result.FirmwareVersion = devInfo.FirmwareVersion
	result.ChannelCount = devInfo.ChannelCount

	result.DeviceType = c.detectDeviceType(devInfo.Model)
	result.IsDoorbell = c.isDoorbellModel(devInfo.Model)
	result.IsNVR = devInfo.ChannelCount > 1 || c.isNVRModel(devInfo.Model)
	result.IsBattery = c.isBatteryModel(devInfo.Model)

	ability, err := c.GetAbility(ctx, 0)
	if err == nil {
		result.HasPTZ = ability.PTZ || ability.PanTilt
		result.HasTwoWayAudio = ability.TwoWayAudio
		result.HasAudioAlarm = ability.AudioAlarm
	}

	result.HasAIDetection = c.hasAIDetection(devInfo.Model)

	for ch := 0; ch < result.ChannelCount; ch++ {
		chInfo := ChannelInfo{
			Channel: ch,
		}

		encCfg, err := c.GetEncoderConfig(ctx, ch)
		if err == nil {
			chInfo.MainStream = encCfg.MainStream
			chInfo.SubStream = encCfg.SubStream
			chInfo.Codec = encCfg.MainStream.Codec
		}

		chInfo.RTMPMain = c.RTMPStreamURL(ch, "main")
		chInfo.RTMPSub = c.RTMPStreamURL(ch, "sub")
		chInfo.RTSPMain = c.RTSPStreamURL(ch, "main")
		chInfo.RTSPSub = c.RTSPStreamURL(ch, "sub")

		result.Channels = append(result.Channels, chInfo)
	}

	return result, nil
}

func (c *Client) RTMPStreamURL(channel int, stream string) string {
	streamID := fmt.Sprintf("channel%d_%s.bcs", channel, stream)
	return fmt.Sprintf("rtmp://%s:1935/bcs/%s?user=%s&password=%s",
		c.host, streamID, url.QueryEscape(c.username), url.QueryEscape(c.password))
}

func (c *Client) RTSPStreamURL(channel int, stream string) string {
	// Stream suffix: "main" for main stream, "sub" for sub stream
	streamSuffix := "main"
	if stream == "sub" {
		streamSuffix = "sub"
	}
	return fmt.Sprintf("rtsp://%s:%s@%s:554/h264Preview_%02d_%s",
		url.QueryEscape(c.username), url.QueryEscape(c.password), c.host, channel+1, streamSuffix)
}

func (c *Client) detectDeviceType(model string) string {
	model = strings.ToLower(model)
	if strings.Contains(model, "doorbell") {
		return "doorbell"
	}
	if strings.Contains(model, "nvr") || strings.Contains(model, "rlnk") {
		return "nvr"
	}
	if strings.Contains(model, "argus") || strings.Contains(model, "lumus") {
		return "battery_camera"
	}
	if strings.Contains(model, "trackmi") {
		return "ptz_camera"
	}
	if strings.Contains(model, "duo") || strings.Contains(model, "floodlight") {
		return "floodlight_camera"
	}
	return "camera"
}

func (c *Client) isDoorbellModel(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "doorbell")
}

func (c *Client) isNVRModel(model string) bool {
	model = strings.ToLower(model)
	nvrModels := []string{"nvr", "rln8-410", "rln16-410", "rln36"}
	for _, nm := range nvrModels {
		if strings.Contains(model, nm) {
			return true
		}
	}
	return false
}

func (c *Client) isBatteryModel(model string) bool {
	model = strings.ToLower(model)
	batteryModels := []string{"argus", "lumus", "go", "battery"}
	for _, bm := range batteryModels {
		if strings.Contains(model, bm) {
			return true
		}
	}
	return false
}

func (c *Client) hasAIDetection(model string) bool {
	model = strings.ToLower(model)
	noAIModels := []string{"rlc-410", "rlc-420", "e1 zoom", "c1 pro"}
	for _, m := range noAIModels {
		if strings.Contains(model, m) {
			return false
		}
	}
	return true
}

func (c *Client) doRequest(ctx context.Context, commands []apiCommand, useToken bool) ([]apiResponse, error) {
	reqURL := c.apiURL()
	if useToken {
		c.mu.RLock()
		token := c.token
		useBasic := c.useBasicAuth
		c.mu.RUnlock()

		if useBasic {
			// Use URL-based credentials instead of token
			reqURL += "?user=" + url.QueryEscape(c.username) + "&password=" + url.QueryEscape(c.password)
		} else if token != "" {
			reqURL += "?token=" + url.QueryEscape(token)
		}
	}
	return c.doRequestURL(ctx, reqURL, commands)
}

func (c *Client) doRequestURL(ctx context.Context, reqURL string, commands []apiCommand) ([]apiResponse, error) {
	body, err := json.Marshal(commands)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed: %s", resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var responses []apiResponse
	if err := json.Unmarshal(respBody, &responses); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return responses, nil
}

// reolinkErrorMessage translates Reolink API error codes to human-readable messages
func reolinkErrorMessage(code int) string {
	switch code {
	case 1:
		return "invalid credentials - check username and password"
	case 2:
		return "account is locked - too many failed login attempts"
	case 3:
		return "session expired - please try again"
	case 4:
		return "command not supported on this device"
	case 5:
		return "device is busy - try again later"
	case 6:
		return "parameter error - invalid request"
	case 7:
		return "permission denied - account may not have admin access"
	default:
		return fmt.Sprintf("unknown error (code %d)", code)
	}
}

// API types
type apiCommand struct {
	Cmd    string                 `json:"cmd"`
	Action int                    `json:"action"`
	Param  map[string]interface{} `json:"param"`
}

type apiResponse struct {
	Cmd   string      `json:"cmd"`
	Code  int         `json:"code"`
	Value interface{} `json:"value"`
}

type DeviceInfo struct {
	Model           string `json:"model"`
	Name            string `json:"name"`
	Serial          string `json:"serial"`
	FirmwareVersion string `json:"firmware_version"`
	ChannelCount    int    `json:"channel_count"`
}

type Ability struct {
	PTZ         bool `json:"ptz"`
	PanTilt     bool `json:"pan_tilt"`
	AudioAlarm  bool `json:"audio_alarm"`
	TwoWayAudio bool `json:"two_way_audio"`
}

type EncoderConfig struct {
	MainStream StreamConfig `json:"main_stream"`
	SubStream  StreamConfig `json:"sub_stream"`
}

type StreamConfig struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	FrameRate int    `json:"frame_rate"`
	BitRate   int    `json:"bit_rate"`
	Codec     string `json:"codec"`
}

type PTZCmd struct {
	Operation string `json:"operation"`
	Speed     int    `json:"speed"`
	Preset    string `json:"preset"`
}

type CameraProbeResult struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Model           string        `json:"model"`
	Name            string        `json:"name"`
	Serial          string        `json:"serial"`
	FirmwareVersion string        `json:"firmware_version"`
	DeviceType      string        `json:"device_type"`
	IsDoorbell      bool          `json:"is_doorbell"`
	IsNVR           bool          `json:"is_nvr"`
	IsBattery       bool          `json:"is_battery"`
	HasPTZ          bool          `json:"has_ptz"`
	HasTwoWayAudio  bool          `json:"has_two_way_audio"`
	HasAudioAlarm   bool          `json:"has_audio_alarm"`
	HasAIDetection  bool          `json:"has_ai_detection"`
	ChannelCount    int           `json:"channel_count"`
	Channels        []ChannelInfo `json:"channels"`
}

type ChannelInfo struct {
	Channel    int          `json:"channel"`
	Name       string       `json:"name,omitempty"`
	Codec      string       `json:"codec"`
	MainStream StreamConfig `json:"main_stream"`
	SubStream  StreamConfig `json:"sub_stream"`
	RTMPMain   string       `json:"rtmp_main"`
	RTMPSub    string       `json:"rtmp_sub"`
	RTSPMain   string       `json:"rtsp_main"`
	RTSPSub    string       `json:"rtsp_sub"`
}

// Utility for snapshots
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
