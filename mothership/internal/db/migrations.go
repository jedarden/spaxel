// Package db provides schema migrations for the Spaxel mothership database.
package db

import (
	"database/sql"
)

// AllMigrations returns the complete list of schema migrations in order.
func AllMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Description: "initial schema",
			Up:          migration_001_initial_schema,
		},
		{
			Version:     2,
			Description: "add diurnal_baselines table",
			Up:          migration_002_add_diurnal_baselines,
		},
		{
			Version:     3,
			Description: "add anomaly_patterns table",
			Up:          migration_003_add_anomaly_patterns,
		},
		{
			Version:     4,
			Description: "add prediction_models table",
			Up:          migration_004_add_prediction_models,
		},
		{
			Version:     5,
			Description: "add ble_device_aliases table",
			Up:          migration_005_add_ble_device_aliases,
		},
		{
			Version:     6,
			Description: "add virtual node columns for passive radar AP",
			Up:          migration_006_add_virtual_node_columns,
		},
		{
			Version:     7,
			Description: "add webhook_log, trigger_state tables and trigger error columns",
			Up:          migration_007_add_webhook_tables,
		},
		{
			Version:     8,
			Description: "add breathing anomaly columns to sleep_records",
			Up:          migration_008_add_breathing_anomaly,
		},
		{
			Version:     9,
			Description: "add unique constraint on sleep_records person+date",
			Up:          migration_009_sleep_records_unique,
		},
		{
			Version:     10,
			Description: "add floorplan table for image upload and calibration",
			Up:          migration_010_add_floorplan,
		},
		{
			Version:     11,
			Description: "add FTS5 table and triggers for events search",
			Up:          migration_011_add_events_fts,
		},
		{
			Version:     12,
			Description: "add crowd flow visualization tables",
			Up:          migration_012_add_crowd_flow_tables,
		},
		{
			Version:     13,
			Description: "add person and sections_json columns to briefings table",
			Up:          migration_013_add_briefing_person_columns,
		},
		{
			Version:     14,
			Description: "add id, delivered, acknowledged columns to briefings table",
			Up:          migration_014_add_briefing_delivery_columns,
		},
		{
			Version:     15,
			Description: "add feature_notifications table for feature discovery",
			Up:          migration_015_add_feature_notifications,
		},
	}
}

// migration_001_initial_schema creates the initial database schema.
func migration_001_initial_schema(tx *sql.Tx) error {
	schema := `
	-- Schema version tracking
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version     INTEGER PRIMARY KEY,
		applied_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		description TEXT
	);

	-- System settings (key-value with typed values)
	CREATE TABLE IF NOT EXISTS settings (
		key         TEXT PRIMARY KEY,
		value_json  TEXT NOT NULL,
		updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Installation secrets and auth
	CREATE TABLE IF NOT EXISTS auth (
		id              INTEGER PRIMARY KEY CHECK (id = 1),
		install_secret  BLOB NOT NULL,
		pin_bcrypt      TEXT,
		updated_at      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Dashboard sessions
	CREATE TABLE IF NOT EXISTS sessions (
		session_id  TEXT PRIMARY KEY,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		expires_at  INTEGER NOT NULL,
		last_seen_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

	-- Node registry
	CREATE TABLE IF NOT EXISTS nodes (
		mac             TEXT PRIMARY KEY,
		node_id         TEXT UNIQUE,
		name            TEXT NOT NULL DEFAULT '',
		pos_x           REAL NOT NULL DEFAULT 0,
		pos_y           REAL NOT NULL DEFAULT 0,
		pos_z           REAL NOT NULL DEFAULT 1,
		role            TEXT NOT NULL DEFAULT 'tx_rx' CHECK (role IN ('tx','rx','tx_rx','passive','idle')),
		firmware_version TEXT,
		chip            TEXT,
		flash_mb        INTEGER,
		capabilities    TEXT,
		status          TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('online','stale','offline')),
		last_seen_ms    INTEGER,
		uptime_ms       INTEGER,
		wifi_rssi_dbm   INTEGER,
		free_heap_bytes INTEGER,
		temperature_c   REAL,
		ip              TEXT,
		created_at      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		updated_at      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Per-link Fresnel zone weights
	CREATE TABLE IF NOT EXISTS link_weights (
		link_id     TEXT PRIMARY KEY,
		weight      REAL NOT NULL DEFAULT 1.0,
		sample_count INTEGER NOT NULL DEFAULT 0,
		updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Baseline snapshots
	CREATE TABLE IF NOT EXISTS baselines (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		link_id     TEXT NOT NULL,
		captured_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		n_sub       INTEGER NOT NULL,
		amplitude   BLOB NOT NULL,
		phase       BLOB NOT NULL,
		confidence  REAL NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_baselines_link ON baselines(link_id, captured_at DESC);

	-- BLE device registry
	CREATE TABLE IF NOT EXISTS ble_devices (
		addr        TEXT PRIMARY KEY,
		label       TEXT NOT NULL DEFAULT '',
		type        TEXT NOT NULL DEFAULT 'person' CHECK (type IN ('person','pet','object')),
		color       TEXT NOT NULL DEFAULT '#888888',
		icon        TEXT,
		auto_rotate INTEGER NOT NULL DEFAULT 0,
		first_seen  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		last_seen   INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		last_rssi   INTEGER,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Floor plan definition
	CREATE TABLE IF NOT EXISTS floorplan (
		id              INTEGER PRIMARY KEY CHECK (id = 1),
		image_path      TEXT,
		cal_ax          REAL,
		cal_ay          REAL,
		cal_bx          REAL,
		cal_by          REAL,
		distance_m      REAL,
		rotation_deg    REAL,
		updated_at      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Zones
	CREATE TABLE IF NOT EXISTS zones (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL UNIQUE,
		x REAL,
		y REAL,
		z REAL,
		w REAL,
		d REAL,
		h REAL,
		zone_type   TEXT NOT NULL DEFAULT 'general'
					CHECK (zone_type IN ('general','bedroom','bathroom','living','exercise','kitchen','office','entry')),
		last_known_occupancy INTEGER NOT NULL DEFAULT 0,
		occupancy_updated_at INTEGER,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Portals
	CREATE TABLE IF NOT EXISTS portals (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		zone_a_id   INTEGER REFERENCES zones(id) ON DELETE SET NULL,
		zone_b_id   INTEGER REFERENCES zones(id) ON DELETE SET NULL,
		points_json TEXT NOT NULL,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Portal crossing log
	CREATE TABLE IF NOT EXISTS portal_crossings (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		portal_id   INTEGER NOT NULL REFERENCES portals(id) ON DELETE CASCADE,
		timestamp_ms INTEGER NOT NULL,
		direction   TEXT NOT NULL CHECK (direction IN ('a_to_b','b_to_a')),
		blob_id     INTEGER,
		person      TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_crossings_portal ON portal_crossings(portal_id, timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_crossings_time ON portal_crossings(timestamp_ms DESC);

	-- Trigger volumes
	CREATE TABLE IF NOT EXISTS triggers (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		shape_json  TEXT NOT NULL,
		condition   TEXT NOT NULL CHECK (condition IN ('enter','leave','dwell','vacant','count')),
		condition_params_json TEXT,
		time_constraint_json TEXT,
		actions_json TEXT NOT NULL,
		enabled     INTEGER NOT NULL DEFAULT 1,
		last_fired  INTEGER,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Events
	CREATE TABLE IF NOT EXISTS events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp_ms INTEGER NOT NULL,
		type        TEXT NOT NULL,
		zone        TEXT,
		person      TEXT,
		blob_id     INTEGER,
		detail_json TEXT,
		severity    TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','alert','critical'))
	);
	CREATE INDEX IF NOT EXISTS idx_events_time ON events(timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_events_zone ON events(zone, timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_events_person ON events(person, timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_events_type ON events(type, timestamp_ms DESC);

	-- Events archive (same schema as events; holds events older than 90 days)
	CREATE TABLE IF NOT EXISTS events_archive (
		id          INTEGER PRIMARY KEY,
		timestamp_ms INTEGER NOT NULL,
		type        TEXT NOT NULL,
		zone        TEXT,
		person      TEXT,
		blob_id     INTEGER,
		detail_json TEXT,
		severity    TEXT NOT NULL DEFAULT 'info'
	);
	CREATE INDEX IF NOT EXISTS idx_events_archive_time ON events_archive(timestamp_ms DESC);

	-- Detection feedback
	CREATE TABLE IF NOT EXISTS feedback (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp_ms INTEGER NOT NULL,
		type        TEXT NOT NULL CHECK (type IN ('correct','incorrect','missed')),
		blob_id     INTEGER,
		position_json TEXT,
		links_json  TEXT,
		event_id    INTEGER REFERENCES events(id) ON DELETE SET NULL
	);
	CREATE INDEX IF NOT EXISTS idx_feedback_time ON feedback(timestamp_ms DESC);

	-- Sleep records
	CREATE TABLE IF NOT EXISTS sleep_records (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		person          TEXT,
		zone_id         INTEGER REFERENCES zones(id) ON DELETE SET NULL,
		date            TEXT NOT NULL,
		bed_time_ms     INTEGER,
		wake_time_ms    INTEGER,
		duration_min    INTEGER,
		onset_latency_min INTEGER,
		restlessness    REAL,
		breathing_rate_avg REAL,
		breathing_regularity REAL,
		summary_json    TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_sleep_person ON sleep_records(person, date DESC);

	-- OTA firmware metadata
	CREATE TABLE IF NOT EXISTS firmware (
		filename        TEXT PRIMARY KEY,
		version         TEXT NOT NULL,
		sha256          TEXT NOT NULL,
		size_bytes      INTEGER NOT NULL,
		uploaded_at     INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		is_latest       INTEGER NOT NULL DEFAULT 0
	);

	-- Morning briefing records
	CREATE TABLE IF NOT EXISTS briefings (
		date        TEXT PRIMARY KEY,
		content     TEXT NOT NULL,
		generated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Notification channel config
	CREATE TABLE IF NOT EXISTS notification_channels (
		type        TEXT PRIMARY KEY,
		enabled     INTEGER NOT NULL DEFAULT 0,
		config_json TEXT NOT NULL DEFAULT '{}'
	);

	-- CSI replay session state
	CREATE TABLE IF NOT EXISTS replay_sessions (
		session_id  TEXT PRIMARY KEY,
		from_ms     INTEGER NOT NULL,
		to_ms       INTEGER NOT NULL,
		current_ms  INTEGER NOT NULL,
		speed       INTEGER NOT NULL DEFAULT 1,
		state       TEXT NOT NULL DEFAULT 'paused' CHECK (state IN ('playing','paused','stopped')),
		params_json TEXT,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);

	-- Crowd flow accumulator
	CREATE TABLE IF NOT EXISTS crowd_flow (
		bucket_ms   INTEGER NOT NULL,
		bucket_type TEXT NOT NULL CHECK (bucket_type IN ('hour','day','week')),
		cell_x      INTEGER NOT NULL,
		cell_y      INTEGER NOT NULL,
		entry_count INTEGER NOT NULL DEFAULT 0,
		vx_sum      REAL NOT NULL DEFAULT 0,
		vy_sum      REAL NOT NULL DEFAULT 0,
		dwell_ms    INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (bucket_ms, bucket_type, cell_x, cell_y)
	);

	-- Initialize with default settings
	INSERT OR IGNORE INTO settings (key, value_json) VALUES
		('fusion_rate_hz', '10'),
		('grid_cell_m', '0.2'),
		('delta_rms_threshold', '0.02'),
		('tau_s', '30'),
		('fresnel_decay', '2.0'),
		('n_subcarriers', '16'),
		('breathing_sensitivity', '0.005'),
		('motion_threshold', '0.05');

	-- Initialize auth with placeholder install secret (replaced by Go on first run)
	INSERT OR IGNORE INTO auth (id, install_secret) VALUES (1, X'0000000000000000000000000000000000000000000000000000000000000000');
	`

	_, err := tx.Exec(schema)
	return err
}

// migration_002_add_diurnal_baselines adds the diurnal baselines table.
func migration_002_add_diurnal_baselines(tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS diurnal_baselines (
		link_id         TEXT NOT NULL,
		hour_of_day     INTEGER NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),
		n_sub           INTEGER NOT NULL,
		amplitude       BLOB NOT NULL,
		phase           BLOB NOT NULL,
		sample_count    INTEGER NOT NULL DEFAULT 0,
		confidence      REAL NOT NULL DEFAULT 0,
		updated_at      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		PRIMARY KEY (link_id, hour_of_day)
	);
	`
	_, err := tx.Exec(schema)
	return err
}

// migration_003_add_anomaly_patterns adds the anomaly detection pattern table.
func migration_003_add_anomaly_patterns(tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS anomaly_patterns (
		zone_id     INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		hour_of_day INTEGER NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),
		day_of_week INTEGER NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
		mean_count  REAL NOT NULL DEFAULT 0,
		variance    REAL NOT NULL DEFAULT 0,
		sample_count INTEGER NOT NULL DEFAULT 0,
		updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		PRIMARY KEY (zone_id, hour_of_day, day_of_week)
	);
	`
	_, err := tx.Exec(schema)
	return err
}

// migration_004_add_prediction_models adds the presence prediction models table.
func migration_004_add_prediction_models(tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS prediction_models (
		person      TEXT NOT NULL,
		zone_id     INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		time_slot   INTEGER NOT NULL,
		day_type    TEXT NOT NULL CHECK (day_type IN ('weekday','weekend')),
		probability REAL NOT NULL DEFAULT 0,
		sample_count INTEGER NOT NULL DEFAULT 0,
		updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		PRIMARY KEY (person, zone_id, time_slot, day_type)
	);
	`
	_, err := tx.Exec(schema)
	return err
}

// migration_005_add_ble_device_aliases adds the BLE device aliases table.
func migration_005_add_ble_device_aliases(tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS ble_device_aliases (
		addr           TEXT NOT NULL,
		canonical_addr TEXT NOT NULL REFERENCES ble_devices(addr) ON DELETE CASCADE,
		first_seen     INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		last_seen      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		PRIMARY KEY (addr)
	);
	CREATE INDEX IF NOT EXISTS idx_ble_aliases_canonical ON ble_device_aliases(canonical_addr);
	`
	_, err := tx.Exec(schema)
	return err
}

// migration_006_add_virtual_node_columns adds columns for virtual AP nodes.
func migration_006_add_virtual_node_columns(tx *sql.Tx) error {
	schema := `
	ALTER TABLE nodes ADD COLUMN virtual INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE nodes ADD COLUMN node_type TEXT NOT NULL DEFAULT 'esp32'
		CHECK (node_type IN ('esp32','ap'));
	ALTER TABLE nodes ADD COLUMN ap_bssid TEXT;
	ALTER TABLE nodes ADD COLUMN ap_channel INTEGER;
	`
	_, err := tx.Exec(schema)
	return err
}


// migration_007_add_webhook_tables adds webhook_log, trigger_state tables
// and error_message/error_count columns to the triggers table.
func migration_007_add_webhook_tables(tx *sql.Tx) error {
	cols := []struct {
		name string
		ddl  string
	}{
		{"error_message", "ALTER TABLE triggers ADD COLUMN error_message TEXT DEFAULT ''"},
		{"error_count", "ALTER TABLE triggers ADD COLUMN error_count INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range cols {
		var exists bool
		if err := tx.QueryRow(
			`SELECT COUNT(*) > 0 FROM pragma_table_info('triggers') WHERE name = ?`, c.name,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(c.ddl); err != nil {
				return err
			}
		}
	}

	_, err := tx.Exec(`
	CREATE TABLE IF NOT EXISTS trigger_state (
		trigger_id  INTEGER NOT NULL,
		blob_id     INTEGER NOT NULL,
		inside      INTEGER NOT NULL DEFAULT 0,
		enter_time  INTEGER NOT NULL DEFAULT 0,
		last_check  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (trigger_id, blob_id),
		FOREIGN KEY (trigger_id) REFERENCES triggers(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS webhook_log (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		trigger_id  INTEGER NOT NULL,
		fired_at_ms INTEGER NOT NULL,
		url         TEXT NOT NULL,
		status_code INTEGER,
		latency_ms  INTEGER NOT NULL DEFAULT 0,
		error       TEXT DEFAULT '',
		FOREIGN KEY (trigger_id) REFERENCES triggers(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_webhook_log_trigger ON webhook_log(trigger_id, fired_at_ms DESC);
	`)
	return err
}

// migration_008_add_breathing_anomaly adds breathing anomaly tracking columns to sleep_records.
func migration_008_add_breathing_anomaly(tx *sql.Tx) error {
	_, err := tx.Exec(`
		ALTER TABLE sleep_records ADD COLUMN breathing_anomaly INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE sleep_records ADD COLUMN breathing_samples_json TEXT;
	`)
	return err
}

// migration_009_sleep_records_unique adds a unique index on (person, date)
// so that the ON CONFLICT upsert in Save() works correctly.
func migration_009_sleep_records_unique(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_sleep_person_date_unique ON sleep_records(person, date)`)
	return err
}

// migration_010_add_floorplan updates the floorplan table schema for
// uploaded floor plan images and pixel-to-meter calibration data.
// For databases with the old schema (cal_distance_m, room_bounds_json),
// it adds the new columns (distance_m, rotation_deg).
func migration_010_add_floorplan(tx *sql.Tx) error {
	// Check if distance_m column already exists (indicates correct schema)
	var colExists bool
	err := tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('floorplan') WHERE name = 'distance_m'
	`).Scan(&colExists)
	if err != nil {
		return err
	}

	// If distance_m doesn't exist, we have the old schema - add new columns
	if !colExists {
		_, err = tx.Exec(`ALTER TABLE floorplan ADD COLUMN distance_m REAL`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`ALTER TABLE floorplan ADD COLUMN rotation_deg REAL`)
		if err != nil {
			return err
		}
	}
	return nil
}

// migration_011_add_events_fts adds FTS5 full-text search for events.
func migration_011_add_events_fts(tx *sql.Tx) error {
	schema := `
	-- FTS5 index for natural-language search across event detail
	CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
		type, zone, person, detail_json,
		content='events', content_rowid='id'
	);

	-- Triggers to keep events_fts in sync with the events table
	CREATE TRIGGER IF NOT EXISTS events_fts_insert AFTER INSERT ON events BEGIN
		INSERT INTO events_fts(rowid, type, zone, person, detail_json)
		VALUES (new.id, new.type, new.zone, new.person, new.detail_json);
	END;

	CREATE TRIGGER IF NOT EXISTS events_fts_delete AFTER DELETE ON events BEGIN
		INSERT INTO events_fts(events_fts, rowid, type, zone, person, detail_json)
		VALUES ('delete', old.id, old.type, old.zone, old.person, old.detail_json);
	END;

	CREATE TRIGGER IF NOT EXISTS events_fts_update AFTER UPDATE ON events BEGIN
		INSERT INTO events_fts(events_fts, rowid, type, zone, person, detail_json)
		VALUES ('delete', old.id, old.type, old.zone, old.person, old.detail_json);
		INSERT INTO events_fts(rowid, type, zone, person, detail_json)
		VALUES (new.id, new.type, new.zone, new.person, new.detail_json);
	END;
	`
	_, err := tx.Exec(schema)
	return err
}

// migration_012_add_crowd_flow_tables adds tables for crowd flow visualization.
func migration_012_add_crowd_flow_tables(tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS trajectory_segments (
		id        TEXT PRIMARY KEY,
		person_id TEXT,
		from_x    REAL NOT NULL,
		from_y    REAL NOT NULL,
		from_z    REAL NOT NULL,
		to_x      REAL NOT NULL,
		to_y      REAL NOT NULL,
		to_z      REAL NOT NULL,
		speed     REAL NOT NULL,
		timestamp DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_traj_timestamp ON trajectory_segments(timestamp);
	CREATE INDEX IF NOT EXISTS idx_traj_person ON trajectory_segments(person_id, timestamp);

	CREATE TABLE IF NOT EXISTS dwell_accumulator (
		grid_x      INTEGER NOT NULL,
		grid_y      INTEGER NOT NULL,
		person_id   TEXT,
		count       INTEGER NOT NULL DEFAULT 1,
		dwell_ms    INTEGER NOT NULL DEFAULT 100,
		last_updated DATETIME NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
		PRIMARY KEY (grid_x, grid_y, person_id)
	);
	CREATE INDEX IF NOT EXISTS idx_dwell_updated ON dwell_accumulator(last_updated);

	CREATE TABLE IF NOT EXISTS detected_corridors (
		id                TEXT PRIMARY KEY,
		centroid_x        REAL NOT NULL,
		centroid_y        REAL NOT NULL,
		centroid_z        REAL NOT NULL,
		direction_x       REAL NOT NULL,
		direction_y       REAL NOT NULL,
		length_m          REAL NOT NULL,
		width_m           REAL NOT NULL,
		cell_count        INTEGER NOT NULL,
		last_computed     DATETIME NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
	);
	`
	_, err := tx.Exec(schema)
	return err
}

// migration_013_add_briefing_person_columns adds person and sections_json columns to briefings table.
func migration_013_add_briefing_person_columns(tx *sql.Tx) error {
	// Check if person column already exists
	var colExists bool
	err := tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'person'
	`).Scan(&colExists)
	if err != nil {
		return err
	}

	// Add columns if they don't exist
	if !colExists {
		_, err = tx.Exec(`ALTER TABLE briefings ADD COLUMN person TEXT`)
		if err != nil {
			return err
		}
	}

	// Add sections_json column for structured briefing data
	var sectionsColExists bool
	err = tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'sections_json'
	`).Scan(&sectionsColExists)
	if err != nil {
		return err
	}

	if !sectionsColExists {
		_, err = tx.Exec(`ALTER TABLE briefings ADD COLUMN sections_json TEXT`)
		if err != nil {
			return err
		}
	}

	return nil
}

// migration_014_add_briefing_delivery_columns adds id, delivered, acknowledged columns to briefings table.
func migration_014_add_briefing_delivery_columns(tx *sql.Tx) error {
	// Add id column (UUID) - primary key replacement
	// Note: We can't add a PRIMARY KEY to an existing table with data, so we'll add a unique index instead
	var idColExists bool
	err := tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'id'
	`).Scan(&idColExists)
	if err != nil {
		return err
	}

	if !idColExists {
		_, err = tx.Exec(`ALTER TABLE briefings ADD COLUMN id TEXT`)
		if err != nil {
			return err
		}
		// Create unique index on id
		_, err = tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_briefings_id ON briefings(id)`)
		if err != nil {
			return err
		}
	}

	// Add delivered column
	var deliveredColExists bool
	err = tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'delivered'
	`).Scan(&deliveredColExists)
	if err != nil {
		return err
	}

	if !deliveredColExists {
		_, err = tx.Exec(`ALTER TABLE briefings ADD COLUMN delivered INTEGER NOT NULL DEFAULT 0`)
		if err != nil {
			return err
		}
	}

	// Add acknowledged column
	var acknowledgedColExists bool
	err = tx.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'acknowledged'
	`).Scan(&acknowledgedColExists)
	if err != nil {
		return err
	}

	if !acknowledgedColExists {
		_, err = tx.Exec(`ALTER TABLE briefings ADD COLUMN acknowledged INTEGER NOT NULL DEFAULT 0`)
		if err != nil {
			return err
		}
	}

	return nil
}

// migration_015_add_feature_notifications adds the feature_notifications table
// for one-time feature discovery notifications.
func migration_015_add_feature_notifications(tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS feature_notifications (
		event_id TEXT PRIMARY KEY,
		fired_at INTEGER NOT NULL,
		acknowledged_at INTEGER
	);
	`
	_, err := tx.Exec(schema)
	return err
}
