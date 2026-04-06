// Package apdetector provides automatic detection of router AP BSSID
// to create virtual TX nodes for passive radar mode.
package apdetector

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/oui"
)

const (
	// Minimum percentage of nodes that must report the same BSSID
	// to auto-detect as the router AP (handles mesh networks)
	minAgreementPercent = 0.8
)

// BSSIDReport represents a single node's AP BSSID report
type BSSIDReport struct {
	NodeMAC   string
	APBSSID   string
	APChannel int
	Timestamp time.Time
}

// APInfo holds information about a detected AP
type APInfo struct {
	BSSID         string
	Channel       int
	Manufacturer  string
	ReportCount   int
	TotalNodes    int
	AgreementPct  float64
	LastUpdated   time.Time
}

// Detector manages AP BSSID detection and virtual node creation
type Detector struct {
	db           *sql.DB
	mu           sync.RWMutex
	reports      map[string][]BSSIDReport // keyed by normalized BSSID
	currentAP    *APInfo
	subscribers  []chan APInfo
}

// NewDetector creates a new AP detector
func NewDetector(db *sql.DB) *Detector {
	return &Detector{
		db:          db,
		reports:     make(map[string][]BSSIDReport),
		subscribers: make([]chan APInfo, 0),
	}
}

// ProcessHello processes a hello message from a node and extracts AP info
func (d *Detector) ProcessHello(mac, apBSSID string, apChannel int) error {
	if apBSSID == "" {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	bssid := normalizeBSSID(apBSSID)

	// Add report
	d.reports[bssid] = append(d.reports[bssid], BSSIDReport{
		NodeMAC:   mac,
		APBSSID:   bssid,
		APChannel: apChannel,
		Timestamp: now,
	})

	// Prune old reports (older than 5 minutes)
	d.pruneReports(now)

	// Check if we have a new dominant AP
	if newAP := d.detectDominantAP(); newAP != nil {
		// Check if AP changed
		if d.currentAP == nil || d.currentAP.BSSID != newAP.BSSID {
			if d.currentAP != nil {
				log.Printf("[INFO] apdetector: AP changed from %s to %s",
					d.currentAP.BSSID, newAP.BSSID)
				d.emitAPChangeAlert(d.currentAP, newAP)
			} else {
				log.Printf("[INFO] apdetector: Detected new AP: %s (%s)",
					newAP.BSSID, newAP.Manufacturer)
			}

			d.currentAP = newAP
			d.currentAP.LastUpdated = now

			// Create or update virtual node
			if err := d.upsertVirtualNode(newAP); err != nil {
				log.Printf("[ERROR] apdetector: Failed to upsert virtual node: %v", err)
			}

			// Notify subscribers
			d.notifySubscribers(*newAP)
		}
	}

	return nil
}

// GetCurrentAP returns the currently detected AP
func (d *Detector) GetCurrentAP() *APInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentAP
}

// Subscribe returns a channel that receives AP updates
func (d *Detector) Subscribe() chan APInfo {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch := make(chan APInfo, 1)
	d.subscribers = append(d.subscribers, ch)

	// Send current state if available
	if d.currentAP != nil {
		ch <- *d.currentAP
	}

	return ch
}

// Unsubscribe removes a subscriber channel
func (d *Detector) Unsubscribe(ch chan APInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, sub := range d.subscribers {
		if sub == ch {
			d.subscribers = append(d.subscribers[:i], d.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// pruneReports removes reports older than 5 minutes
func (d *Detector) pruneReports(now time.Time) {
	cutoff := now.Add(-5 * time.Minute)

	for bssid, reports := range d.reports {
		filtered := make([]BSSIDReport, 0, len(reports))
		for _, r := range reports {
			if r.Timestamp.After(cutoff) {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(d.reports, bssid)
		} else {
			d.reports[bssid] = filtered
		}
	}
}

// detectDominantAP analyzes all reports and returns the dominant AP if found
func (d *Detector) detectDominantAP() *APInfo {
	if len(d.reports) == 0 {
		return nil
	}

	// Count total unique nodes
	totalNodes := make(map[string]bool)
	for _, reports := range d.reports {
		for _, r := range reports {
			totalNodes[r.NodeMAC] = true
		}
	}

	totalNodeCount := len(totalNodes)
	if totalNodeCount == 0 {
		return nil
	}

	// Find BSSID with highest agreement
	var bestBSSID string
	var bestCount int

	for bssid, reports := range d.reports {
		// Count unique nodes reporting this BSSID
		uniqueNodes := make(map[string]bool)
		for _, r := range reports {
			uniqueNodes[r.NodeMAC] = true
		}
		count := len(uniqueNodes)

		if count > bestCount {
			bestCount = count
			bestBSSID = bssid
		}
	}

	// Check if we meet the minimum agreement threshold
	agreementPct := float64(bestCount) / float64(totalNodeCount)
	if agreementPct < minAgreementPercent {
		return nil
	}

	// Build AP info
	reports := d.reports[bestBSSID]
	channel := reports[0].APChannel

	// Look up manufacturer via OUI
	macBytes := bssidToBytes(bestBSSID)
	manufacturer := oui.LookupOUI(macBytes)
	if manufacturer == "" {
		manufacturer = "Unknown Router"
	}

	return &APInfo{
		BSSID:        bestBSSID,
		Channel:      channel,
		Manufacturer: manufacturer,
		ReportCount:  bestCount,
		TotalNodes:   totalNodeCount,
		AgreementPct: agreementPct,
	}
}

// upsertVirtualNode creates or updates a virtual node for the AP
func (d *Detector) upsertVirtualNode(ap *APInfo) error {
	mac := ap.BSSID
	name := fmt.Sprintf("%s Router", ap.Manufacturer)

	nowMs := time.Now().UnixNano() / 1e6 // Convert to milliseconds

	// Use INSERT OR REPLACE to handle both new and existing virtual nodes
	// Note: updated_at will be set automatically by DEFAULT
	query := `
		INSERT INTO nodes (mac, name, role, pos_x, pos_y, pos_z, virtual, node_type, ap_bssid, ap_channel, last_seen_ms, created_at, updated_at)
		VALUES (?, ?, 'ap', 0, 0, 2.5, 1, 'ap', ?, ?, ?, ?, ?)
		ON CONFLICT(mac) DO UPDATE SET
			name = excluded.name,
			ap_bssid = excluded.ap_bssid,
			ap_channel = excluded.ap_channel,
			last_seen_ms = excluded.last_seen_ms,
			updated_at = excluded.updated_at,
			virtual = 1,
			node_type = 'ap'
	`

	_, err := d.db.Exec(query, mac, name, ap.BSSID, ap.Channel, nowMs, nowMs, nowMs)
	return err
}

// emitAPChangeAlert logs an event and can be extended to send notifications
func (d *Detector) emitAPChangeAlert(oldAP, newAP *APInfo) {
	// Log to events table for timeline
	detail := map[string]interface{}{
		"old_bssid":    oldAP.BSSID,
		"new_bssid":    newAP.BSSID,
		"old_channel":  oldAP.Channel,
		"new_channel":  newAP.Channel,
		"manufacturer": newAP.Manufacturer,
	}

	detailJSON, _ := json.Marshal(detail)

	_, err := d.db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, detail_json, severity)
		VALUES (?, 'ap_changed', 'system', ?, 'warning')
	`, time.Now().UnixNano(), string(detailJSON))

	if err != nil {
		log.Printf("[WARN] apdetector: Failed to log AP change event: %v", err)
	}
}

// notifySubscribers sends AP updates to all subscribers
func (d *Detector) notifySubscribers(ap APInfo) {
	for _, ch := range d.subscribers {
		select {
		case ch <- ap:
		default:
			// Channel full, skip
		}
	}
}

// normalizeBSSID converts a MAC address to uppercase colon-separated format
func normalizeBSSID(bssid string) string {
	// Remove any existing separators
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			return r
		}
		return -1
	}, bssid)

	// Convert to uppercase
	cleaned = strings.ToUpper(cleaned)

	// Add colons: AA:BB:CC:DD:EE:FF
	if len(cleaned) != 12 {
		return bssid
	}

	return cleaned[0:2] + ":" + cleaned[2:4] + ":" + cleaned[4:6] + ":" +
		cleaned[6:8] + ":" + cleaned[8:10] + ":" + cleaned[10:12]
}

// bssidToBytes converts a colon-separated BSSID to 6 bytes
func bssidToBytes(bssid string) []byte {
	parts := strings.Split(bssid, ":")
	if len(parts) != 6 {
		return nil
	}

	bytes := make([]byte, 6)
	for i, part := range parts {
		var b uint8
		fmt.Sscanf(part, "%x", &b)
		bytes[i] = b
	}

	return bytes
}
