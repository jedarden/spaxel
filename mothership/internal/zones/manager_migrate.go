package zones

// migrate creates the database schema. Split from manager.go for editability.
func (m *Manager) migrate() error {
	_, err := m.db.Exec(`
			CREATE TABLE IF NOT EXISTS zones (
				id        TEXT PRIMARY KEY,
				name     TEXT    NOT NULL DEFAULT '',
				color    TEXT    NOT NULL DEFAULT '#4fc3f7',
				min_x     REAL    NOT NULL DEFAULT 0,
				min_y     REAL    NOT NULL DEFAULT 0,
				min_z     REAL    NOT NULL DEFAULT 0,
				max_x     REAL    NOT NULL DEFAULT 1,
				max_y     REAL    NOT NULL DEFAULT 1,
				max_z     REAL    NOT NULL DEFAULT 1,
				enabled  INTEGER NOT NULL DEFAULT 1,
				zone_type TEXT   NOT NULL DEFAULT 'normal',
				is_children_zone INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL DEFAULT 0
			);

			CREATE TABLE IF NOT EXISTS portals (
				id        TEXT PRIMARY KEY,
				name     TEXT    NOT NULL DEFAULT '',
				zone_a_id TEXT    NOT NULL DEFAULT '',
				zone_b_id TEXT    NOT NULL DEFAULT '',
				p1_x      REAL    NOT NULL DEFAULT 0,
				p1_y      REAL    NOT NULL DEFAULT 0,
				p1_z      REAL    NOT NULL DEFAULT 0,
				p2_x      REAL    NOT NULL DEFAULT 0,
				p2_y      REAL    NOT NULL DEFAULT 0,
				p2_z      REAL    NOT NULL DEFAULT 0,
				p3_x      REAL    NOT NULL DEFAULT 0,
				p3_y      REAL    NOT NULL DEFAULT 0,
				p3_z      REAL    NOT NULL DEFAULT 0,
				n_x      REAL    NOT NULL DEFAULT 0,
				n_y      REAL    NOT NULL DEFAULT 0,
				n_z      REAL    NOT NULL DEFAULT 0,
				width    REAL    NOT NULL DEFAULT 1,
				height   REAL    NOT NULL DEFAULT 2,
				enabled  INTEGER NOT NULL DEFAULT 1,
				created_at INTEGER NOT NULL DEFAULT 0
			);

			CREATE TABLE IF NOT EXISTS crossing_events (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				portal_id   TEXT    NOT NULL,
				blob_id     INTEGER NOT NULL,
				direction  INTEGER NOT NULL,
				from_zone  TEXT    NOT NULL,
				to_zone    TEXT    NOT NULL,
				timestamp  INTEGER NOT NULL,
				identity   TEXT    DEFAULT ''
			);

			CREATE INDEX IF NOT EXISTS idx_crossing_time ON crossing_events(timestamp);

			CREATE TABLE IF NOT EXISTS zone_history (
				id        INTEGER PRIMARY KEY AUTOINCREMENT,
				zone_id  TEXT    NOT NULL,
				hour_ts  INTEGER NOT NULL,
				count    INTEGER NOT NULL,
				people   TEXT    NOT NULL DEFAULT '[]',
				created_at INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
				UNIQUE(zone_id, hour_ts)
			);
			CREATE INDEX IF NOT EXISTS idx_zone_history_zone_time ON zone_history(zone_id, hour_ts DESC);
		`)
	if err != nil {
		return err
	}

	// Add zone_type column if it doesn't exist (migration for existing databases)
	m.db.Exec(`ALTER TABLE zones ADD COLUMN zone_type TEXT NOT NULL DEFAULT 'normal'`)
	m.db.Exec(`ALTER TABLE zones ADD COLUMN is_children_zone INTEGER NOT NULL DEFAULT 0`)

	// Add last_known_occupancy column for restart reconciliation
	m.db.Exec(`ALTER TABLE zones ADD COLUMN last_known_occupancy INTEGER NOT NULL DEFAULT 0`)
	m.db.Exec(`ALTER TABLE zones ADD COLUMN occupancy_updated_at INTEGER`)

	return nil
}
