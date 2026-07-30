package ota

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// ErrFirmwareNotFound is returned when the requested firmware version or
// filename is not present, so callers can map it to 404 rather than a generic
// 500. See ADR-004 / bf-2cb85.
var ErrFirmwareNotFound = errors.New("firmware not found")

// NodeOTAState tracks where a node is in the OTA update lifecycle.
type NodeOTAState int

const (
	OTAIdle        NodeOTAState = iota
	OTAPending                  // queued for update
	OTADownloading              // node is downloading firmware
	OTARebooting                // node rebooted into new partition
	OTAVerified                 // node reconnected with new version
	OTAFailed                   // download or verification failed
	OTARollback                 // node came back with old version
)

func (s NodeOTAState) String() string {
	switch s {
	case OTAIdle:
		return "idle"
	case OTAPending:
		return "pending"
	case OTADownloading:
		return "downloading"
	case OTARebooting:
		return "rebooting"
	case OTAVerified:
		return "verified"
	case OTAFailed:
		return "failed"
	case OTARollback:
		return "rollback"
	default:
		return "unknown"
	}
}

// NodeOTAProgress tracks per-node OTA progress.
type NodeOTAProgress struct {
	MAC             string
	State           NodeOTAState
	ProgressPct     uint8
	Error           string
	ExpectedVersion string
	PreviousVersion string
	UpdatedAt       time.Time
}

// NodeSender can send OTA commands to connected nodes.
type NodeSender interface {
	SendOTAToMAC(mac, url, sha256, version string)
	GetConnectedMACs() []string
}

// DashboardBroadcaster can broadcast OTA progress updates to dashboard clients.
type DashboardBroadcaster interface {
	BroadcastOTAProgress(mac, state string, progressPct uint8, expectedVersion, previousVersion, errorMsg string)
}

// Manager orchestrates rolling OTA updates across the fleet.
type Manager struct {
	mu          sync.RWMutex
	server      *Server
	sender      NodeSender
	broadcaster DashboardBroadcaster
	progress    map[string]*NodeOTAProgress
	baseURL     string // e.g. "http://mothership:8080"
}

// NewManager creates an OTA manager.
// baseURL is the HTTP base URL from which firmware is served (e.g. "http://mothership:8080").
func NewManager(srv *Server, baseURL string) *Manager {
	return &Manager{
		server:   srv,
		progress: make(map[string]*NodeOTAProgress),
		baseURL:  baseURL,
	}
}

// SetSender sets the node sender (wired to the ingestion server).
func (m *Manager) SetSender(s NodeSender) {
	m.mu.Lock()
	m.sender = s
	m.mu.Unlock()
}

// SetDashboardBroadcaster sets the dashboard broadcaster for real-time progress updates.
func (m *Manager) SetDashboardBroadcaster(b DashboardBroadcaster) {
	m.mu.Lock()
	m.broadcaster = b
	m.mu.Unlock()
}

// GetProgress returns the current OTA progress map (a snapshot copy).
func (m *Manager) GetProgress() map[string]NodeOTAProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]NodeOTAProgress, len(m.progress))
	for k, v := range m.progress {
		out[k] = *v
	}
	return out
}

// SendOTA triggers an OTA update for a single node.
// Uses the latest available firmware.
func (m *Manager) SendOTA(mac string) error {
	meta := m.server.GetLatest()
	if meta == nil {
		return fmt.Errorf("%w: no firmware uploaded", ErrFirmwareNotFound)
	}
	return m.sendOTAWithMeta(mac, meta)
}

// SendOTAVersion triggers an OTA update for a single node using a specific
// firmware, identified by either its version string or its filename.
//
// The API passes a version, but this previously looked up by filename only, so
// a version that GET /api/firmware listed as present resolved to "not found".
// Accept both: version first (what callers actually supply), then filename for
// backwards compatibility. See ADR-004 / bf-2cb85.
func (m *Manager) SendOTAVersion(mac, versionOrFilename string) error {
	meta := m.server.GetByVersion(versionOrFilename)
	if meta == nil {
		meta = m.server.GetByFilename(versionOrFilename)
	}
	if meta == nil {
		return fmt.Errorf("%w: %q", ErrFirmwareNotFound, versionOrFilename)
	}
	return m.sendOTAWithMeta(mac, meta)
}

func (m *Manager) sendOTAWithMeta(mac string, meta *FirmwareMeta) error {
	m.mu.RLock()
	sender := m.sender
	m.mu.RUnlock()

	if sender == nil {
		return fmt.Errorf("sender not configured")
	}

	// Refuse rather than hand the node a URL it cannot fetch. Previously the
	// base URL was derived from the bind address, so a default deployment sent
	// "http://0.0.0.0:8080/firmware/..." and the node failed to connect while
	// the API reported success. See ADR-004 / bf-2f0uu.
	if m.baseURL == "" {
		return fmt.Errorf("OTA unavailable: no advertised base URL configured " +
			"(set SPAXEL_ADVERTISED_BASE_URL to an address reachable from the nodes)")
	}

	url := fmt.Sprintf("%s/firmware/%s", m.baseURL, meta.Filename)

	m.mu.Lock()
	p := m.progress[mac]
	if p == nil {
		p = &NodeOTAProgress{MAC: mac}
		m.progress[mac] = p
	}
	p.State = OTAPending
	p.ExpectedVersion = meta.Version
	p.UpdatedAt = time.Now()

	// Broadcast pending state to dashboard
	if m.broadcaster != nil {
		m.broadcaster.BroadcastOTAProgress(mac, "pending", 0, meta.Version, p.PreviousVersion, "")
	}

	m.mu.Unlock()

	sender.SendOTAToMAC(mac, url, meta.SHA256, meta.Version)
	log.Printf("[INFO] ota: triggered update on %s → %s (sha256=%s)", mac, meta.Version, meta.SHA256)
	return nil
}

// SendOTAAll runs a rolling update of all connected nodes.
// Sends to nodes one at a time with rollingGap between each.
// Halts if more than 50% of the fleet goes offline during the update.
func (m *Manager) SendOTAAll(ctx context.Context, rollingGap time.Duration) error {
	m.mu.RLock()
	sender := m.sender
	m.mu.RUnlock()

	if sender == nil {
		return fmt.Errorf("sender not configured")
	}

	meta := m.server.GetLatest()
	if meta == nil {
		return fmt.Errorf("%w: no firmware uploaded", ErrFirmwareNotFound)
	}

	macs := sender.GetConnectedMACs()
	if len(macs) == 0 {
		return fmt.Errorf("no connected nodes")
	}

	totalNodes := len(macs)
	log.Printf("[INFO] ota: rolling update of %d nodes to %s", totalNodes, meta.Version)

	for i, mac := range macs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := m.sendOTAWithMeta(mac, meta); err != nil {
			log.Printf("[WARN] ota: failed to trigger %s: %v", mac, err)
			continue
		}

		// Safety check: halt if >50% of fleet is offline after this node reboots
		if i < totalNodes-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rollingGap):
			}

			connected := len(sender.GetConnectedMACs())
			if totalNodes > 1 && connected < totalNodes/2 {
				return fmt.Errorf("halted: >50%% of fleet offline (%d/%d connected)", connected, totalNodes)
			}
		}
	}

	log.Printf("[INFO] ota: rolling update dispatched to %d nodes", totalNodes)
	return nil
}

// OnOTAStatus is called by the ingestion server when a node sends an ota_status message.
func (m *Manager) OnOTAStatus(mac, state string, progressPct uint8, errMsg string) {
	m.mu.Lock()
	p := m.progress[mac]
	if p == nil {
		p = &NodeOTAProgress{MAC: mac}
		m.progress[mac] = p
	}

	switch state {
	case "downloading":
		p.State = OTADownloading
		p.ProgressPct = progressPct
	case "verifying":
		p.State = OTADownloading
		p.ProgressPct = progressPct
	case "rebooting":
		p.State = OTARebooting
		p.ProgressPct = 100
	case "failed":
		p.State = OTAFailed
		p.Error = errMsg
	}
	p.UpdatedAt = time.Now()

	// Broadcast progress to dashboard if broadcaster is set
	if m.broadcaster != nil {
		m.broadcaster.BroadcastOTAProgress(mac, p.State.String(), progressPct, p.ExpectedVersion, p.PreviousVersion, errMsg)
	}

	m.mu.Unlock()

	log.Printf("[INFO] ota: %s status=%s pct=%d err=%q", mac, state, progressPct, errMsg)
}

// OnNodeReconnected is called when a node sends hello after an OTA attempt.
// Detects whether the update was applied or rolled back.
func (m *Manager) OnNodeReconnected(mac, firmwareVersion string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.progress[mac]
	if !ok {
		return
	}

	if p.State != OTARebooting {
		return
	}

	var broadcastState string
	if firmwareVersion == p.ExpectedVersion {
		p.State = OTAVerified
		broadcastState = "verified"
		log.Printf("[INFO] ota: %s verified new firmware %s", mac, firmwareVersion)
	} else {
		p.State = OTARollback
		broadcastState = "rollback"
		log.Printf("[WARN] ota: %s rolled back to %s (expected %s)", mac, firmwareVersion, p.ExpectedVersion)
	}
	p.UpdatedAt = time.Now()

	// Broadcast final state to dashboard
	if m.broadcaster != nil {
		m.broadcaster.BroadcastOTAProgress(mac, broadcastState, 100, p.ExpectedVersion, firmwareVersion, "")
	}
}
