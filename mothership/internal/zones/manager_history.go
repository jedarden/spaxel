package zones

import (
	"encoding/json"
	"log"
	"time"
)

// RecordZoneHistorySnapshot records the current occupancy for all zones as an hourly snapshot.
// Should be called periodically (e.g., every hour) to build historical occupancy data.
// The hour_ts is the Unix millisecond timestamp of the start of the hour bucket.
func (m *Manager) RecordZoneHistorySnapshot() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Calculate the start of the current hour bucket in local timezone
	now := time.Now().In(m.tz)
	hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, m.tz)
	hourTs := hourStart.UnixMilli()

	// Record a snapshot for each zone
	for zoneID := range m.zones {
		occ := m.occupancy[zoneID]
		count := 0
		var people []string
		if occ != nil {
			count = occ.Count
			// Note: people list not yet implemented - would need blob identity resolver
			people = []string{}
		}

		peopleJSON, err := json.Marshal(people)
		if err != nil {
			peopleJSON = []byte("[]")
		}

		// UPSERT into zone_history (ON CONFLICT UPDATE for existing hour_ts)
		_, err = m.db.Exec(`
			INSERT INTO zone_history (zone_id, hour_ts, count, people, created_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(zone_id, hour_ts) DO UPDATE SET
				count = excluded.count,
				people = excluded.people,
				created_at = excluded.created_at
		`, zoneID, hourTs, count, string(peopleJSON), time.Now().UnixMilli())
		if err != nil {
			log.Printf("[WARN] Failed to record zone history snapshot for zone %s: %v", zoneID, err)
		}
	}
}
