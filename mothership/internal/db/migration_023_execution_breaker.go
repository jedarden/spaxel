package db

import "database/sql"

// migration_023_add_execution_breaker adds the durable account-local breaker
// state and its append-only transition history. The state row is retained
// after a rearm so the last trip and rearm remain available for audit.
func migration_023_add_execution_breaker(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS execution_breaker_states (
			account_id          TEXT PRIMARY KEY,
			state               TEXT NOT NULL CHECK (state IN ('active','reduce_only','halted')),
			reason              TEXT NOT NULL DEFAULT '',
			triggered_by        TEXT NOT NULL DEFAULT '',
			triggered_kind      TEXT NOT NULL DEFAULT '',
			triggered_at        INTEGER NOT NULL DEFAULT 0,
			rearmed_by          TEXT NOT NULL DEFAULT '',
			rearmed_kind        TEXT NOT NULL DEFAULT '',
			rearmed_at          INTEGER NOT NULL DEFAULT 0,
			revision            INTEGER NOT NULL CHECK (revision > 0),
			updated_at          INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS execution_breaker_events (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id          TEXT NOT NULL,
			previous_state      TEXT NOT NULL CHECK (previous_state IN ('active','reduce_only','halted')),
			state               TEXT NOT NULL CHECK (state IN ('active','reduce_only','halted')),
			event_kind          TEXT NOT NULL CHECK (event_kind IN ('reduce_only','halt','rearm')),
			actor_id            TEXT NOT NULL,
			actor_kind          TEXT NOT NULL,
			reason              TEXT NOT NULL DEFAULT '',
			created_at          INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_execution_breaker_events_account
			ON execution_breaker_events(account_id, id);
	`)
	return err
}
