// Package main provides scenario simulation modes for acceptance testing.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// ScenarioType defines the type of scenario to simulate
type ScenarioType string

const (
	ScenarioNormal     ScenarioType = "normal"
	ScenarioFall       ScenarioType = "fall"
	ScenarioOTA        ScenarioType = "ota"
	ScenarioBagOnCouch ScenarioType = "bag-on-couch"
)

// ScenarioConfig holds scenario-specific configuration
type ScenarioConfig struct {
	Type       ScenarioType
	FallParams FallScenarioParams
	OTAParams  OTAScenarioParams
	StartedAt  time.Time
	Phase      string // for multi-phase scenarios
}

// FallScenarioParams defines parameters for fall detection scenario
type FallScenarioParams struct {
	TriggerAfter      time.Duration // Time before fall triggers
	DescentDuration   time.Duration // How long the fall takes
	StillnessDuration time.Duration // How long to stay still after fall
	MinVelocity       float64       // Minimum Z velocity (m/s, negative for falling)
	MinZDrop          float64       // Minimum Z drop (meters)
	EndZ              float64       // Final Z height (meters, typically floor level)
}

// OTAScenarioParams defines parameters for OTA update scenario
type OTAScenarioParams struct {
	UpdateAfter      time.Duration // Time before OTA starts
	FirmwareSize     int64         // Size of firmware in bytes
	NewVersion       string        // New firmware version
	RebootDelay      time.Duration // Delay before rebooting
	BootFailDuration time.Duration // How long to simulate boot failure (for rollback test)
	SimulateFailure  bool          // Whether to simulate a boot failure
}

// FallScenarioState tracks fall scenario state for a walker
type FallScenarioState struct {
	Walker          *Walker
	State           string // "walking", "falling", "on_floor", "recovering"
	FallStartTime   time.Time
	PreFallPosition Point
	PreFallVelocity Point
}

// updateWalkerForFallScenario updates walker position for fall scenario
func (s *FallScenarioState) UpdateForFallScenario(dt float64, params FallScenarioParams, space *Space, rng *rand.Rand) {
	switch s.State {
	case "walking":
		// Normal walking behavior
		s.Walker.Position.X += s.Walker.Velocity.X * dt
		s.Walker.Position.Y += s.Walker.Velocity.Y * dt

		// Bounce off walls
		margin := 0.2
		if s.Walker.Position.X < margin {
			s.Walker.Position.X = margin
			s.Walker.Velocity.X *= -1
		}
		if s.Walker.Position.X > space.Width-margin {
			s.Walker.Position.X = space.Width - margin
			s.Walker.Velocity.X *= -1
		}
		if s.Walker.Position.Y < margin {
			s.Walker.Position.Y = margin
			s.Walker.Velocity.Y *= -1
		}
		if s.Walker.Position.Y > space.Depth-margin {
			s.Walker.Position.Y = space.Depth - margin
			s.Walker.Velocity.Y *= -1
		}

		// Random velocity perturbation
		perturbation := 0.1
		s.Walker.Velocity.X += (rng.Float64() - 0.5) * perturbation
		s.Walker.Velocity.Y += (rng.Float64() - 0.5) * perturbation

		// Clamp velocity
		speed := s.Walker.Speed * (0.5 + rng.Float64()*0.5)
		currentSpeed := math.Sqrt(s.Walker.Velocity.X*s.Walker.Velocity.X + s.Walker.Velocity.Y*s.Walker.Velocity.Y)
		if currentSpeed > 0 {
			s.Walker.Velocity.X = (s.Walker.Velocity.X / currentSpeed) * speed
			s.Walker.Velocity.Y = (s.Walker.Velocity.Y / currentSpeed) * speed
		}

		s.Walker.Position.Z = s.Walker.Height

	case "falling":
		// Rapid Z descent with high downward velocity
		elapsed := time.Since(s.FallStartTime).Seconds()
		progress := elapsed / params.DescentDuration.Seconds()

		if progress >= 1.0 {
			// Fall complete
			s.State = "on_floor"
			s.Walker.Position.Z = params.EndZ
			s.Walker.Velocity.X = 0
			s.Walker.Velocity.Y = 0
			s.Walker.Velocity.Z = 0
			log.Printf("[SIM] Fall complete - Z now at %.2f m", s.Walker.Position.Z)
		} else {
			// Animate fall
			zDrop := s.PreFallPosition.Z - params.EndZ
			s.Walker.Position.Z = s.PreFallPosition.Z - zDrop*progress

			// Downward velocity exceeds threshold
			s.Walker.Velocity.Z = -math.Abs(params.MinVelocity) - 0.5 // Add margin

			// Slight forward motion during fall
			s.Walker.Position.X += s.PreFallVelocity.X * dt * 0.5
			s.Walker.Position.Y += s.PreFallVelocity.Y * dt * 0.5
		}

	case "on_floor":
		// Stay still on floor - no motion
		s.Walker.Position.Z = params.EndZ
		s.Walker.Velocity.X = 0
		s.Walker.Velocity.Y = 0
		s.Walker.Velocity.Z = 0

	case "recovering":
		// Quick recovery (for false positive test)
		s.Walker.Position.Z += 0.5 * dt // Stand up quickly
		if s.Walker.Position.Z >= s.Walker.Height {
			s.Walker.Position.Z = s.Walker.Height
			s.State = "walking"
		}
	}
}

// StartFall triggers the fall sequence
func (s *FallScenarioState) StartFall(params FallScenarioParams) {
	s.PreFallPosition = s.Walker.Position
	s.PreFallVelocity = s.Walker.Velocity
	s.FallStartTime = time.Now()
	s.State = "falling"
	log.Printf("[SIM] Triggering fall from Z=%.2f m with velocity %.2f m/s",
		s.Walker.Position.Z, params.MinVelocity)
}

// OTAScenarioState tracks OTA scenario state for a node
type OTAScenarioState struct {
	Node            *VirtualNode
	State           string // "idle", "downloading", "installing", "rebooting", "updated", "rollback"
	CurrentVersion  string
	DownloadedBytes int64
	DownloadStart   time.Time
	RebootStart     time.Time
	FailureStart    time.Time
	AllNodes        []*VirtualNode
}

// SendOTAStatus sends OTA status message to mothership
func (s *OTAScenarioState) SendOTAStatus(ctx context.Context) error {
	status := map[string]interface{}{
		"type":             "ota_status",
		"mac":              macToString(s.Node.MAC),
		"timestamp_ms":     time.Now().UnixMilli(),
		"state":            s.State,
		"current_version":  s.CurrentVersion,
		"downloaded_bytes": s.DownloadedBytes,
	}

	msgBytes, err := json.Marshal(status)
	if err != nil {
		return err
	}

	s.Node.mu.Lock()
	defer s.Node.mu.Unlock()
	return s.Node.Conn.WriteMessage(websocket.TextMessage, msgBytes)
}

// SimulateOTADownload simulates the firmware download process
func (s *OTAScenarioState) SimulateOTADownload(ctx context.Context, params OTAScenarioParams, progress chan<- float64) error {
	s.State = "downloading"
	s.DownloadStart = time.Now()

	chunkSize := int64(4096) // 4KB chunks
	totalChunks := (params.FirmwareSize + chunkSize - 1) / chunkSize

	for i := int64(0); i < totalChunks; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Simulate download delay (100ms per chunk)
		time.Sleep(100 * time.Millisecond)

		s.DownloadedBytes = (i + 1) * chunkSize
		if s.DownloadedBytes > params.FirmwareSize {
			s.DownloadedBytes = params.FirmwareSize
		}

		pct := float64(s.DownloadedBytes) / float64(params.FirmwareSize)
		if progress != nil {
			select {
			case progress <- pct:
			default:
			}
		}

		// Send status every 25%
		if i%(totalChunks/4) == 0 || i == totalChunks-1 {
			if err := s.SendOTAStatus(ctx); err != nil {
				return err
			}
			log.Printf("[SIM] Node %d OTA download: %.1f%% (%d/%d bytes)",
				s.Node.ID, pct*100, s.DownloadedBytes, params.FirmwareSize)
		}
	}

	s.State = "installing"
	if err := s.SendOTAStatus(ctx); err != nil {
		return err
	}

	return nil
}

// SimulateOTAInstall simulates firmware installation
func (s *OTAScenarioState) SimulateOTAInstall(ctx context.Context, params OTAScenarioParams) error {
	log.Printf("[SIM] Node %d installing firmware %s...", s.Node.ID, params.NewVersion)

	// Simulate installation time (2 seconds)
	time.Sleep(2 * time.Second)

	s.CurrentVersion = params.NewVersion
	s.State = "rebooting"
	s.RebootStart = time.Now()

	if err := s.SendOTAStatus(ctx); err != nil {
		return err
	}

	return nil
}

// SimulateOTAReboot simulates the reboot process
func (s *OTAScenarioState) SimulateOTAReboot(ctx context.Context, params OTAScenarioParams) error {
	log.Printf("[SIM] Node %d rebooting...", s.Node.ID)

	// Send goodbye
	s.Node.mu.Lock()
	s.Node.Conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "rebooting"))
	s.Node.Conn.Close()
	s.Node.mu.Unlock()

	// Simulate reboot delay
	time.Sleep(params.RebootDelay)

	if params.SimulateFailure {
		// Simulate boot failure
		log.Printf("[SIM] Node %d simulating boot failure...", s.Node.ID)
		s.State = "rollback"
		s.FailureStart = time.Now()
		time.Sleep(params.BootFailDuration)

		// Rollback to previous version
		s.CurrentVersion = "sim-1.0.0"
		log.Printf("[SIM] Node %d rolled back to %s", s.Node.ID, s.CurrentVersion)
	} else {
		// Successful reboot
		s.State = "updated"
		log.Printf("[SIM] Node %d reboot complete, version %s", s.Node.ID, s.CurrentVersion)
	}

	return nil
}

// reconnectNode reconnects a node to mothership after reboot
func reconnectNode(ctx context.Context, node *VirtualNode, allNodes []*VirtualNode) error {
	// Reuse connection logic from main.go
	token := *flagToken
	if token == "" {
		var err error
		token, err = provisionToken()
		if err != nil {
			return err
		}
	}

	wsURL, err := url.Parse(*flagMothership)
	if err != nil {
		return err
	}

	if wsURL.Scheme == "http" {
		wsURL.Scheme = "ws"
	} else if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	}

	headers := http.Header{}
	headers.Set("X-Spaxel-Token", token)

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL.String(), headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial failed: %w (status %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("dial failed: %w", err)
	}

	node.Conn = conn

	// Send hello with new version
	hello := map[string]interface{}{
		"type":             "hello",
		"mac":              macToString(node.MAC),
		"firmware_version": "sim-1.1.0",
		"capabilities":     []string{"csi", "tx", "rx"},
		"chip":             "ESP32-S3",
		"flash_mb":         16,
		"uptime_ms":        1000,
		"wifi_rssi":        -45,
		"ip":               fmt.Sprintf("127.0.0.%d", node.ID+2),
	}

	helloBytes, _ := json.Marshal(hello)
	node.mu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, helloBytes)
	node.mu.Unlock()

	if err != nil {
		conn.Close()
		return err
	}

	// Wait for role assignment
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return err
	}

	var roleMsg map[string]interface{}
	json.Unmarshal(message, &roleMsg)

	log.Printf("[SIM] Node %d reconnected, role: %v", node.ID, roleMsg["role"])

	return nil
}
