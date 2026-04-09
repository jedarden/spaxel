// Package dashboard handles WebSocket connections from dashboard clients
// and broadcasts CSI data from the ingestion server.
package dashboard

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/replay"
	"github.com/spaxel/mothership/internal/tracking"
)

// Hub manages all dashboard client connections and broadcasts
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client

	// Reference to ingestion server for state queries
	ingestionState IngestionState

	// Additional state providers
	bleState        BLEState
	triggerState    TriggerState
	systemHealth    SystemHealthProvider
	zoneState       ZoneStateProvider
	eventStore      EventStore
	securityState   SecurityStateProvider
	sleepState      SleepStateProvider

	// Pending events buffer — events accumulated between 10 Hz delta ticks.
	pendingEvents   []map[string]interface{}
	pendingEventsMu sync.Mutex

	// Snapshot protocol: stores the last full snapshot for delta computation.
	// Updated on every 10 Hz tick.
	snapMu sync.RWMutex
	snap   snapshotCache
}

// snapshotCache holds serialised JSON bytes for each snapshot field,
// allowing cheap byte-level comparison when computing deltas.
type snapshotCache struct {
	blobsJSON         []byte
	nodesJSON         []byte
	zonesJSON         []byte
	portalsJSON       []byte
	linksJSON         []byte
	bleJSON           []byte
	triggersJSON      []byte
	motionStatesJSON  []byte
	securityJSON      []byte
	confidence        int
	timestampMs       int64
}

// ZoneStateProvider is an interface to query zone data for the dashboard snapshot.
type ZoneStateProvider interface {
	GetAllZones() []ZoneSnapshot
	GetAllPortals() []PortalSnapshot
	GetOccupancy() map[string]ZoneOccupancySnapshot
	GetOccupancyStatus() map[string]string
}

// PortalSnapshot is the wire format for a portal in the dashboard snapshot.
type PortalSnapshot struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ZoneA     string  `json:"zone_a"`
	ZoneB     string  `json:"zone_b"`
	P1X       float64 `json:"p1_x"`
	P1Y       float64 `json:"p1_y"`
	P1Z       float64 `json:"p1_z"`
	P2X       float64 `json:"p2_x"`
	P2Y       float64 `json:"p2_y"`
	P2Z       float64 `json:"p2_z"`
	P3X       float64 `json:"p3_x"`
	P3Y       float64 `json:"p3_y"`
	P3Z       float64 `json:"p3_z"`
	NX        float64 `json:"n_x"`
	NY        float64 `json:"n_y"`
	NZ        float64 `json:"n_z"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Enabled   bool    `json:"enabled"`
}

// ZoneSnapshot is the wire format for a zone in the dashboard snapshot.
type ZoneSnapshot struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Count       int      `json:"count"`
	People      []string `json:"people"`
	MinX        float64  `json:"x"`
	MinY        float64  `json:"y"`
	MinZ        float64  `json:"z"`
	SizeX       float64  `json:"w"`
	SizeY       float64  `json:"d"`
	SizeZ       float64  `json:"h"`
	OccStatus   string   `json:"occ_status,omitempty"` // "uncertain" or "reconciled"
}

// ZoneOccupancySnapshot provides occupancy counts for zones.
type ZoneOccupancySnapshot struct {
	Count   int    `json:"count"`
	BlobIDs []int  `json:"blob_ids"`
}

// IngestionState is an interface to query node/link/motion state from ingestion
type IngestionState interface {
	GetConnectedNodesInfo() []ingestion.NodeInfo
	GetAllLinksInfo() []ingestion.LinkInfo
	GetAllMotionStates() []ingestion.MotionStateItem
}

// BLEState is an interface to query current BLE devices for dashboard broadcast
type BLEState interface {
	GetCurrentDevices() []map[string]interface{}
}

// TriggerState is an interface to query automation trigger states for dashboard broadcast
type TriggerState interface {
	GetTriggerStates() []map[string]interface{}
}

// SystemHealthProvider is an interface to query system health metrics
type SystemHealthProvider interface {
	GetUptimeSeconds() int64
	GetNodeCount() int
	GetBeadCount() int
	GetGoRoutineCount() int
	GetMemoryMB() float64
}

// EventStore is an interface for persisting events to the database.
type EventStore interface {
	LogEvent(eventType string, timestamp time.Time, zone, person string, blobID int, detailJSON, severity string) error
}

// SecurityStateProvider provides security mode state for the dashboard snapshot.
type SecurityStateProvider interface {
	IsSecurityModeActive() bool
	GetSecurityMode() string
	GetLearningProgress() float64
	IsModelReady() bool
}

// SleepStateProvider provides sleep monitoring state for morning summary push.
type SleepStateProvider interface {
	ShouldPushMorningSummary() (bool, map[string]interface{})
}

// ZoneChangeBroadcaster notifies dashboard clients when zones or portals
// are created, updated, or deleted via the REST API. Implementations should
// both send an immediate typed broadcast and invalidate the snapshot cache
// so the next delta tick doesn't send stale data.
type ZoneChangeBroadcaster interface {
	BroadcastZoneChange(action string, zone ZoneSnapshot)
	BroadcastPortalChange(action string, portal PortalSnapshot)
}

// Client represents a dashboard WebSocket client
type Client struct {
	hub  *Hub
	send chan []byte
}

// NewHub creates a new dashboard hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// SetIngestionState sets the ingestion state provider
func (h *Hub) SetIngestionState(state IngestionState) {
	h.mu.Lock()
	h.ingestionState = state
	h.mu.Unlock()
}

// SetBLEState sets the BLE state provider
func (h *Hub) SetBLEState(state BLEState) {
	h.mu.Lock()
	h.bleState = state
	h.mu.Unlock()
}

// SetTriggerState sets the automation trigger state provider
func (h *Hub) SetTriggerState(state TriggerState) {
	h.mu.Lock()
	h.triggerState = state
	h.mu.Unlock()
}

// SetSystemHealth sets the system health provider
func (h *Hub) SetSystemHealth(provider SystemHealthProvider) {
	h.mu.Lock()
	h.systemHealth = provider
	h.mu.Unlock()
}

// SetEventStore sets the event persistence store
func (h *Hub) SetEventStore(store EventStore) {
	h.mu.Lock()
	h.eventStore = store
	h.mu.Unlock()
}

// SetZoneState sets the zone state provider for snapshot broadcasts.
func (h *Hub) SetZoneState(state ZoneStateProvider) {
	h.mu.Lock()
	h.zoneState = state
	h.mu.Unlock()
}

// SetSecurityState sets the security state provider for snapshot broadcasts.
func (h *Hub) SetSecurityState(state SecurityStateProvider) {
	h.mu.Lock()
	h.securityState = state
	h.mu.Unlock()
}

// SetSleepState sets the sleep state provider for morning summary push.
func (h *Hub) SetSleepState(state SleepStateProvider) {
	h.mu.Lock()
	h.sleepState = state
	h.mu.Unlock()
}

// Run starts the hub's main loop.
// The 10 Hz delta tick replaces the old 5 s state / 500 ms presence broadcasts.
// BLE scan results are broadcast every 5 s as a separate typed message.
// System health (60 s) is kept as a separate low-frequency broadcast.
func (h *Hub) Run() {
	// 10 Hz snapshot/delta tick
	deltaTicker := time.NewTicker(100 * time.Millisecond)
	defer deltaTicker.Stop()

	// System health broadcast ticker (60 seconds) — kept separate
	healthTicker := time.NewTicker(60 * time.Second)
	defer healthTicker.Stop()

	// BLE scan broadcast ticker (5 seconds)
	bleScanTicker := time.NewTicker(5 * time.Second)
	defer bleScanTicker.Stop()

	for {
		select {
		case client := <-h.register:
			// Build and send snapshot BEFORE adding the client to the
			// broadcast map so that no delta messages race ahead of the
			// initial state.
			snap := h.buildSnapshot()
			data, err := json.Marshal(snap)
			if err != nil {
				log.Printf("[WARN] Failed to marshal snapshot: %v", err)
			} else {
				select {
				case client.send <- data:
				default:
					log.Printf("[WARN] Snapshot dropped for new client (buffer full)")
				}
			}

			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
			log.Printf("[INFO] Dashboard client connected (total: %d)", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[INFO] Dashboard client disconnected (total: %d)", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, skip
				}
			}
			h.mu.RUnlock()

		case <-deltaTicker.C:
			h.tickDelta()

		case <-healthTicker.C:
			h.broadcastSystemHealth()

		case <-bleScanTicker.C:
			h.broadcastBLEScan()
		}
	}
}

// Register registers a new dashboard client
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister unregisters a dashboard client
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast sends a message to all connected dashboard clients
func (h *Hub) Broadcast(message []byte) {
	select {
	case h.broadcast <- message:
	default:
		// Channel full, drop message
	}
}

// BroadcastCSI broadcasts a CSI frame to all dashboard clients
func (h *Hub) BroadcastCSI(nodeMAC, peerMAC string, data []byte) {
	h.Broadcast(data)
}

// BroadcastNodeConnected notifies dashboards of a new node
func (h *Hub) BroadcastNodeConnected(mac, firmware, chip string) {
	msg := map[string]interface{}{
		"type":             "node_connected",
		"mac":              mac,
		"firmware_version": firmware,
		"chip":             chip,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastNodeDisconnected notifies dashboards of a node leaving
func (h *Hub) BroadcastNodeDisconnected(mac string) {
	msg := map[string]interface{}{
		"type": "node_disconnected",
		"mac":  mac,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastLinkActive notifies dashboards of an active link
func (h *Hub) BroadcastLinkActive(linkID, nodeMAC, peerMAC string) {
	msg := map[string]interface{}{
		"type":     "link_active",
		"id":       linkID,
		"node_mac": nodeMAC,
		"peer_mac": peerMAC,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastLinkInactive notifies dashboards of an inactive link
func (h *Hub) BroadcastLinkInactive(linkID string) {
	msg := map[string]interface{}{
		"type": "link_inactive",
		"id":   linkID,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastMotionState sends motion state for one or more links to all dashboard clients.
// Called on state changes (idle↔motion) so the dashboard updates immediately.
func (h *Hub) BroadcastMotionState(states []ingestion.MotionStateItem) {
	msg := map[string]interface{}{
		"type":  "motion_state",
		"links": states,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// ─── Phase 3 Broadcasts ─────────────────────────────────────────────────────

// nodeJSON is the wire format for a fleet node sent to the dashboard.
type nodeJSON struct {
	MAC      string  `json:"mac"`
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	PosX     float64 `json:"pos_x"`
	PosY     float64 `json:"pos_y"`
	PosZ     float64 `json:"pos_z"`
	Virtual  bool    `json:"virtual"`
	LastSeen int64   `json:"last_seen_ms"`
}

// roomJSON is the wire format for room configuration.
type roomJSON struct {
	Width   float64 `json:"width"`
	Depth   float64 `json:"depth"`
	Height  float64 `json:"height"`
	OriginX float64 `json:"origin_x"`
	OriginZ float64 `json:"origin_z"`
}

// BroadcastRegistryState sends updated node registry and room config to all dashboard clients.
func (h *Hub) BroadcastRegistryState(nodes []fleet.NodeRecord, room fleet.RoomConfig) {
	wireNodes := make([]nodeJSON, len(nodes))
	for i, n := range nodes {
		wireNodes[i] = nodeJSON{
			MAC:      n.MAC,
			Name:     n.Name,
			Role:     n.Role,
			PosX:     n.PosX,
			PosY:     n.PosY,
			PosZ:     n.PosZ,
			Virtual:  n.Virtual,
			LastSeen: n.LastSeenAt.UnixMilli(),
		}
	}
	msg := map[string]interface{}{
		"type":  "registry_state",
		"nodes": wireNodes,
		"room": roomJSON{
			Width:   room.Width,
			Depth:   room.Depth,
			Height:  room.Height,
			OriginX: room.OriginX,
			OriginZ: room.OriginZ,
		},
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// trailPoint is a compact [x, z] pair for JSON serialisation.
type trailPoint [2]float64

// blobJSON is the wire format for a tracked person blob.
type blobJSON struct {
	ID                 int          `json:"id"`
	X                  float64      `json:"x"`
	Z                  float64      `json:"z"`
	VX                 float64      `json:"vx"`
	VZ                 float64      `json:"vz"`
	Weight             float64      `json:"weight"`
	Trail              []trailPoint `json:"trail"`
	Posture            string       `json:"posture,omitempty"`
	PersonID           string       `json:"person_id,omitempty"`
	PersonLabel        string       `json:"person_label,omitempty"`
	PersonColor        string       `json:"person_color,omitempty"`
	IdentityConfidence float64      `json:"identity_confidence,omitempty"`
	IdentitySource     string       `json:"identity_source,omitempty"`
}

// BroadcastLocUpdate sends localisation results to all dashboard clients.
// It also stores the latest blob data for the snapshot/delta protocol.
func (h *Hub) BroadcastLocUpdate(blobs []tracking.Blob) {
	wireBlobs := make([]blobJSON, len(blobs))
	for i, b := range blobs {
		trail := make([]trailPoint, len(b.Trail))
		for j, pt := range b.Trail {
			trail[j] = trailPoint{pt[0], pt[1]}
		}
		wireBlobs[i] = blobJSON{
			ID:                 b.ID,
			X:                  b.X,
			Z:                  b.Z,
			VX:                 b.VX,
			VZ:                 b.VZ,
			Weight:             b.Weight,
			Trail:              trail,
			Posture:            string(b.Posture),
			PersonID:           b.PersonID,
			PersonLabel:        b.PersonLabel,
			PersonColor:        b.PersonColor,
			IdentityConfidence: b.IdentityConfidence,
			IdentitySource:     b.IdentitySource,
		}
	}

	// Store for snapshot protocol.
	h.snapMu.Lock()
	if data, err := json.Marshal(wireBlobs); err == nil {
		h.snap.blobsJSON = data
	}
	h.snapMu.Unlock()

	msg := map[string]interface{}{
		"type":  "loc_update",
		"blobs": wireBlobs,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastCoverageMap sends the GDOP coverage map to all dashboard clients.
// data is a row-major float32 array of GDOP values, cols × rows cells.
func (h *Hub) BroadcastCoverageMap(data []float32, cols, rows int, cellSize float64, originX, originZ float64) {
	// Encode as a compact flat array of float32 values (JSON).
	vals := make([]float64, len(data))
	for i, v := range data {
		vals[i] = float64(v)
	}
	msg := map[string]interface{}{
		"type":      "coverage_map",
		"cols":      cols,
		"rows":      rows,
		"cell_size": cellSize,
		"origin_x":  originX,
		"origin_z":  originZ,
		"data":      vals,
	}
	encoded, _ := json.Marshal(msg)
	h.Broadcast(encoded)
}

func (h *Hub) sendInitialState(client *Client) {
	// Legacy path kept for tests that call sendInitialState directly.
	// The Run() loop now handles snapshot delivery on register.
	snap := h.buildSnapshot()
	data, _ := json.Marshal(snap)
	select {
	case client.send <- data:
	default:
	}
}

// buildSnapshot constructs the full snapshot message for a new client connection.
func (h *Hub) buildSnapshot() map[string]interface{} {
	now := time.Now().UnixMilli()
	snap := map[string]interface{}{
		"type":         "snapshot",
		"timestamp_ms": now,
	}

	h.mu.RLock()
	ing := h.ingestionState
	ble := h.bleState
	trig := h.triggerState
	zones := h.zoneState
	h.mu.RUnlock()

	if ing != nil {
		if nodes := ing.GetConnectedNodesInfo(); len(nodes) > 0 {
			snap["nodes"] = nodes
		}
		if links := ing.GetAllLinksInfo(); len(links) > 0 {
			snap["links"] = links
		}
		if motionStates := ing.GetAllMotionStates(); len(motionStates) > 0 {
			snap["motion_states"] = motionStates
		}
	}

	if ble != nil {
		if devices := ble.GetCurrentDevices(); len(devices) > 0 {
			snap["ble_devices"] = devices
		}
	}

	if trig != nil {
		if triggers := trig.GetTriggerStates(); len(triggers) > 0 {
			snap["triggers"] = triggers
		}
	}

	if zones != nil {
		snap["zones"] = h.buildZoneSnapshots(zones)
		if portals := zones.GetAllPortals(); len(portals) > 0 {
			snap["portals"] = portals
		}
	}

	// Include latest blobs from the snapshot cache.
	h.snapMu.RLock()
	if len(h.snap.blobsJSON) > 0 {
		var blobs []blobJSON
		if json.Unmarshal(h.snap.blobsJSON, &blobs) == nil {
			snap["blobs"] = blobs
		}
	}
	h.snapMu.RUnlock()

	return snap
}

// buildZoneSnapshots converts zone state into the wire format for the snapshot.
func (h *Hub) buildZoneSnapshots(zp ZoneStateProvider) []ZoneSnapshot {
	zones := zp.GetAllZones()
	occupancy := zp.GetOccupancy()
	statusMap := zp.GetOccupancyStatus()
	result := make([]ZoneSnapshot, 0, len(zones))
	for _, z := range zones {
		occ, ok := occupancy[z.ID]
		people := make([]string, 0)
		if ok {
			_ = occ.BlobIDs
		}
		occStatus := statusMap[z.ID]
		snap := ZoneSnapshot{
			ID:     z.ID,
			Name:   z.Name,
			Count:  occ.Count,
			People: people,
			MinX:   z.MinX,
			MinY:   z.MinY,
			MinZ:   z.MinZ,
			SizeX:  z.SizeX,
			SizeY:  z.SizeY,
			SizeZ:  z.SizeZ,
		}
		if occStatus == "uncertain" {
			snap.OccStatus = "uncertain"
		}
		result = append(result, snap)
	}
	return result
}

// tickDelta is called every 100 ms (10 Hz). It computes which snapshot
// fields changed since the last tick and broadcasts only those fields.
// Delta messages omit the "type" field so the frontend can distinguish
// them from event-driven messages.
func (h *Hub) tickDelta() {
	h.mu.RLock()
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if clientCount == 0 {
		return
	}

	now := time.Now().UnixMilli()
	delta := make(map[string]interface{})
	delta["timestamp_ms"] = now

	h.mu.RLock()
	ing := h.ingestionState
	ble := h.bleState
	trig := h.triggerState
	zones := h.zoneState
	h.mu.RUnlock()

	// --- blobs (stored by BroadcastLocUpdate) ---
	h.snapMu.Lock()
	if ing != nil {
		if nodes := ing.GetConnectedNodesInfo(); len(nodes) > 0 {
			if data, err := json.Marshal(nodes); err == nil {
				if !bytesEqual(data, h.snap.nodesJSON) {
					delta["nodes"] = nodes
					h.snap.nodesJSON = data
				}
			}
		} else {
			if len(h.snap.nodesJSON) > 0 {
				delta["nodes"] = []ingestion.NodeInfo{}
				h.snap.nodesJSON = nil
			}
		}

		if links := ing.GetAllLinksInfo(); len(links) > 0 {
			if data, err := json.Marshal(links); err == nil {
				if !bytesEqual(data, h.snap.linksJSON) {
					delta["links"] = links
					h.snap.linksJSON = data
				}
			}
		} else {
			if len(h.snap.linksJSON) > 0 {
				delta["links"] = []ingestion.LinkInfo{}
				h.snap.linksJSON = nil
			}
		}

		if motionStates := ing.GetAllMotionStates(); len(motionStates) > 0 {
			if data, err := json.Marshal(motionStates); err == nil {
				if !bytesEqual(data, h.snap.motionStatesJSON) {
					delta["motion_states"] = motionStates
					h.snap.motionStatesJSON = data
				}
			}
		}
	}

	if len(h.snap.blobsJSON) > 0 {
		delta["blobs"] = json.RawMessage(h.snap.blobsJSON)
	}

	if ble != nil {
		if devices := ble.GetCurrentDevices(); len(devices) > 0 {
			if data, err := json.Marshal(devices); err == nil {
				if !bytesEqual(data, h.snap.bleJSON) {
					delta["ble_devices"] = devices
					h.snap.bleJSON = data
				}
			}
		}
	}

	if trig != nil {
		if triggers := trig.GetTriggerStates(); len(triggers) > 0 {
			if data, err := json.Marshal(triggers); err == nil {
				if !bytesEqual(data, h.snap.triggersJSON) {
					delta["triggers"] = triggers
					h.snap.triggersJSON = data
				}
			}
		}
	}

	if zones != nil {
		zs := h.buildZoneSnapshots(zones)
		if data, err := json.Marshal(zs); err == nil {
			if !bytesEqual(data, h.snap.zonesJSON) {
				delta["zones"] = zs
				h.snap.zonesJSON = data
			}
		}

		ps := zones.GetAllPortals()
		if data, err := json.Marshal(ps); err == nil {
			if !bytesEqual(data, h.snap.portalsJSON) {
				delta["portals"] = ps
				h.snap.portalsJSON = data
			}
		}
	}

	h.snap.timestampMs = now
	h.snapMu.Unlock()

	// Include any pending events that arrived since the last tick.
	h.pendingEventsMu.Lock()
	if len(h.pendingEvents) > 0 {
		delta["events"] = h.pendingEvents
		h.pendingEvents = nil
	}
	h.pendingEventsMu.Unlock()

	// Only broadcast if something actually changed (beyond timestamp).
	if len(delta) <= 1 {
		return
	}

	data, err := json.Marshal(delta)
	if err != nil {
		log.Printf("[WARN] Failed to marshal delta: %v", err)
		return
	}
	h.Broadcast(data)
}

// bytesEqual compares two byte slices. Nil and empty are treated as equal.
func bytesEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ClientCount returns the number of connected dashboard clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// broadcastSystemHealth broadcasts system health stats to all dashboard clients.
func (h *Hub) broadcastSystemHealth() {
	h.mu.RLock()
	provider := h.systemHealth
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if provider == nil || clientCount == 0 {
		return
	}

	h.BroadcastSystemHealth(
		provider.GetUptimeSeconds(),
		provider.GetNodeCount(),
		provider.GetBeadCount(),
		provider.GetGoRoutineCount(),
		provider.GetMemoryMB(),
	)
}

// broadcastBLEScan broadcasts the current BLE device list to all dashboard clients.
// Called every 5 seconds when devices are present.
func (h *Hub) broadcastBLEScan() {
	h.mu.RLock()
	ble := h.bleState
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if ble == nil || clientCount == 0 {
		return
	}

	if devices := ble.GetCurrentDevices(); len(devices) > 0 {
		h.BroadcastBLEScan(devices)
	}
}

// BroadcastFleetChange broadcasts a fleet change event to all dashboard clients.
// This implements the fleet.FleetChangeBroadcaster interface.
func (h *Hub) BroadcastFleetChange(event fleet.FleetChangeEvent) {
	msg := map[string]interface{}{
		"type":              "fleet_change",
		"timestamp":         event.Timestamp.UnixMilli(),
		"trigger_reason":    event.TriggerReason,
		"mean_gdop_before":  event.MeanGDOPBefore,
		"mean_gdop_after":   event.MeanGDOPAfter,
		"coverage_before":   event.CoverageBefore,
		"coverage_after":    event.CoverageAfter,
		"coverage_delta":    event.CoverageDelta,
		"is_degradation":    event.IsDegradation,
		"role_assignments":  event.RoleAssignments,
	}

	if event.OfflineMAC != "" {
		msg["offline_mac"] = event.OfflineMAC
	}
	if event.RecoveredMAC != "" {
		msg["recovered_mac"] = event.RecoveredMAC
	}
	if event.WarningMessage != "" {
		msg["warning_message"] = event.WarningMessage
	}
	if len(event.GDOPBefore) > 0 {
		msg["gdop_before"] = floatsToSlice(event.GDOPBefore)
		msg["gdop_after"] = floatsToSlice(event.GDOPAfter)
		msg["gdop_cols"] = event.GDOPCols
		msg["gdop_rows"] = event.GDOPRows
	}

	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// floatsToSlice converts []float32 to []float64 for JSON marshalling
func floatsToSlice(f []float32) []float64 {
	result := make([]float64, len(f))
	for i, v := range f {
		result[i] = float64(v)
	}
	return result
}

// BroadcastFleetHealth broadcasts current fleet health status.
func (h *Hub) BroadcastFleetHealth(nodes []fleet.NodeRecord, roles map[string]string, coverageScore float64) {
	type nodeHealth struct {
		MAC         string  `json:"mac"`
		Name        string  `json:"name"`
		Role        string  `json:"role"`
		HealthScore float64 `json:"health_score"`
		Online      bool    `json:"online"`
	}

	wireNodes := make([]nodeHealth, len(nodes))
	for i, n := range nodes {
		role := n.Role
		if r, ok := roles[n.MAC]; ok {
			role = r
		}
		wireNodes[i] = nodeHealth{
			MAC:         n.MAC,
			Name:        n.Name,
			Role:        role,
			HealthScore: n.HealthScore,
			Online:      n.LastSeenAt.After(time.Now().Add(-5 * time.Minute)),
		}
	}

	msg := map[string]interface{}{
		"type":           "fleet_health",
		"nodes":          wireNodes,
		"coverage_score": coverageScore,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastFleetHistory broadcasts optimisation history to dashboard.
func (h *Hub) BroadcastFleetHistory(history []fleet.OptimisationHistoryRecord) {
	type historyEntry struct {
		ID              int64   `json:"id"`
		Timestamp       int64   `json:"timestamp_ms"`
		TriggerReason   string  `json:"trigger_reason"`
		MeanGDOPBefore  float64 `json:"mean_gdop_before"`
		MeanGDOPAfter   float64 `json:"mean_gdop_after"`
		CoverageDelta   float64 `json:"coverage_delta"`
	}

	wireHistory := make([]historyEntry, len(history))
	for i, rec := range history {
		wireHistory[i] = historyEntry{
			ID:             rec.ID,
			Timestamp:      rec.Timestamp.UnixMilli(),
			TriggerReason:  rec.TriggerReason,
			MeanGDOPBefore: rec.MeanGDOPBefore,
			MeanGDOPAfter:  rec.MeanGDOPAfter,
			CoverageDelta:  rec.CoverageDelta,
		}
	}

	msg := map[string]interface{}{
		"type":    "fleet_history",
		"history": wireHistory,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// FleetChangeEvent is re-exported for compatibility
type FleetChangeEvent = fleet.FleetChangeEvent

// BroadcastSystemModeChange broadcasts a system mode change event to all dashboard clients.
func (h *Hub) BroadcastSystemModeChange(event interface{}) {
	msg := map[string]interface{}{
		"type": "system_mode_change",
		"data": event,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastAnomaly broadcasts an anomaly detection event to all dashboard clients.
func (h *Hub) BroadcastAnomaly(anomaly interface{}) {
	msg := map[string]interface{}{
		"type": "anomaly_detected",
		"data": anomaly,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastEvent broadcasts an event (presence transition, zone entry/exit, portal crossing) to all dashboard clients.
// It also persists the event to the database and buffers it for inclusion in the next incremental update.
func (h *Hub) BroadcastEvent(eventID string, timestamp time.Time, kind, zone string, blobID int, personName string) {
	evt := map[string]interface{}{
		"id":          eventID,
		"ts":          timestamp.UnixMilli(),
		"kind":        kind,
		"zone":        zone,
		"blob_id":     blobID,
		"person_name": personName,
	}

	msg := map[string]interface{}{
		"type":  "event",
		"event": evt,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)

	// Buffer for inclusion in the next incremental update.
	h.pendingEventsMu.Lock()
	h.pendingEvents = append(h.pendingEvents, evt)
	// Keep buffer bounded to prevent unbounded growth.
	if len(h.pendingEvents) > 100 {
		h.pendingEvents = h.pendingEvents[len(h.pendingEvents)-50:]
	}
	h.pendingEventsMu.Unlock()

	// Persist to database if store is configured.
	h.mu.RLock()
	store := h.eventStore
	h.mu.RUnlock()
	if store != nil {
		go func() {
			if err := store.LogEvent(kind, timestamp, zone, personName, blobID, "", "info"); err != nil {
				log.Printf("[WARN] Failed to persist event %s: %v", eventID, err)
			}
		}()
	}
}

// BroadcastAlert broadcasts an alert (anomaly detection, security mode trigger) to all dashboard clients.
func (h *Hub) BroadcastAlert(alertID string, timestamp time.Time, severity, description string, acknowledged bool) {
	msg := map[string]interface{}{
		"type": "alert",
		"alert": map[string]interface{}{
			"id":           alertID,
			"ts":           timestamp.UnixMilli(),
			"severity":     severity,
			"description":  description,
			"acknowledged": acknowledged,
		},
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastBLEScan broadcasts BLE device list updates to all dashboard clients (5s interval).
func (h *Hub) BroadcastBLEScan(devices []map[string]interface{}) {
	msg := map[string]interface{}{
		"type":    "ble_scan",
		"devices": devices,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastTriggerState broadcasts automation trigger state changes to all dashboard clients.
func (h *Hub) BroadcastTriggerState(triggerID, name string, lastFired time.Time, enabled bool) {
	msg := map[string]interface{}{
		"type": "trigger_state",
		"trigger": map[string]interface{}{
			"id":         triggerID,
			"name":       name,
			"last_fired": lastFired.UnixMilli(),
			"enabled":    enabled,
		},
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastSystemHealth broadcasts periodic system health stats to all dashboard clients (60s interval).
func (h *Hub) BroadcastSystemHealth(uptimeS int64, nodeCount, beadCount, goRoutines int, memMB float64) {
	msg := map[string]interface{}{
		"type": "system_health",
		"health": map[string]interface{}{
			"uptime_s":     uptimeS,
			"node_count":   nodeCount,
			"bead_count":   beadCount,
			"go_routines":  goRoutines,
			"mem_mb":       memMB,
		},
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastEventFromDB broadcasts an event from the database to all dashboard clients.
// Field names match BroadcastEvent so the frontend can handle both uniformly:
// { type: "event", event: { id, ts, kind, zone, blob_id, person_name, detail_json, severity } }
func (h *Hub) BroadcastEventFromDB(id int64, timestamp int64, eventType, zone, person string, blobID int, detailJSON, severity string) {
	msg := map[string]interface{}{
		"type": "event",
		"event": map[string]interface{}{
			"id":           id,
			"ts":           timestamp,
			"kind":         eventType,
			"zone":         zone,
			"blob_id":      blobID,
			"person_name":  person,
			"detail_json":  detailJSON,
			"severity":     severity,
		},
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastZoneChange sends an immediate zone change event to all dashboard
// clients and invalidates the cached zone snapshot so the next delta tick
// reflects the new state. action is "created", "updated", or "deleted".
func (h *Hub) BroadcastZoneChange(action string, zone ZoneSnapshot) {
	msg := map[string]interface{}{
		"type":  "zone_change",
		"action": action,
		"zone":  zone,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)

	// Invalidate cached zones snapshot so the next delta tick re-serialises.
	h.snapMu.Lock()
	h.snap.zonesJSON = nil
	h.snapMu.Unlock()
}

// BroadcastPortalChange sends an immediate portal change event to all dashboard
// clients and invalidates the cached portal snapshot. action is "created",
// "updated", or "deleted".
func (h *Hub) BroadcastPortalChange(action string, portal PortalSnapshot) {
	msg := map[string]interface{}{
		"type":   "portal_change",
		"action":  action,
		"portal": portal,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)

	// Invalidate cached portals snapshot.
	h.snapMu.Lock()
	h.snap.portalsJSON = nil
	h.snapMu.Unlock()
}

// BroadcastLoadState broadcasts a load shedding state change to all dashboard clients.
// Used when the system enters or exits heavy load shedding (Level 3).
func (h *Hub) BroadcastLoadState(level int, label string) {
	msg := map[string]interface{}{
		"type":        "alert",
		"severity":    "warning",
		"description": "System load: " + label,
		"load_level":  level,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastMorningSummary pushes a sleep morning summary card to all connected
// dashboard clients. This is fired on the first connection after 6am when a
// completed sleep session exists.
func (h *Hub) BroadcastMorningSummary(summary map[string]interface{}) {
	msg := map[string]interface{}{
		"type":     "morning_summary",
		"sleep":    summary,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastReplayBlobs broadcasts replay blob updates to all dashboard clients.
// This implements the replay.BlobBroadcaster interface for time-travel debugging.
func (h *Hub) BroadcastReplayBlobs(blobs []replay.BlobUpdate, timestampMS int64) {
	wireBlobs := make([]blobJSON, len(blobs))
	for i, b := range blobs {
		trail := make([]trailPoint, len(b.Trail)/2)
		for j := 0; j < len(b.Trail)/2; j++ {
			trail[j] = trailPoint{b.Trail[j*2], b.Trail[j*2+1]}
		}
		wireBlobs[i] = blobJSON{
			ID:                 b.ID,
			X:                  b.X,
			Z:                  b.Z,
			VX:                 b.VX,
			VZ:                 b.VZ,
			Weight:             b.Weight,
			Trail:              trail,
			Posture:            b.Posture,
			PersonID:           b.PersonID,
			PersonLabel:        b.PersonLabel,
			PersonColor:        b.PersonColor,
			IdentityConfidence: b.IdentityConfidence,
			IdentitySource:     b.IdentitySource,
		}
	}

	msg := map[string]interface{}{
		"type":         "replay_update",
		"blobs":        wireBlobs,
		"timestamp_ms": timestampMS,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}
