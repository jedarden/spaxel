package db

import "database/sql"

// migration_022_add_work_planner adds the durable singleton planner lease and
// the queue of work items that helpers claim. Lease rows are retained after
// expiry so a replacement planner always receives a new fencing epoch.
func migration_022_add_work_planner(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS work_planner_leases (
			planner_id  TEXT PRIMARY KEY,
			owner_id    TEXT NOT NULL,
			fence_token INTEGER NOT NULL CHECK (fence_token > 0),
			expires_at  INTEGER NOT NULL,
			acquired_at INTEGER NOT NULL,
			renewed_at  INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS work_items (
			id                TEXT PRIMARY KEY,
			dedupe_key        TEXT NOT NULL UNIQUE,
			work_kind         TEXT NOT NULL,
			payload           BLOB NOT NULL,
			state             TEXT NOT NULL CHECK (state IN ('queued','leased','completed','failed')),
			available_at      INTEGER NOT NULL,
			attempt_count     INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
			attempt_limit     INTEGER NOT NULL DEFAULT 3 CHECK (attempt_limit > 0),
			claim_owner_id    TEXT NOT NULL DEFAULT '',
			claim_token       INTEGER NOT NULL DEFAULT 0 CHECK (claim_token >= 0),
			claim_expires_at  INTEGER NOT NULL DEFAULT 0,
			created_at        INTEGER NOT NULL,
			updated_at        INTEGER NOT NULL,
			completed_at      INTEGER,
			last_error        TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_work_items_available
			ON work_items(state, available_at, claim_expires_at);
	`)
	return err
}
