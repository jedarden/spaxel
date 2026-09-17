package db

import "database/sql"

// migration_021_add_execution_owner_leases adds the durable account-local
// execution ownership and write tables. The owner lease is intentionally
// retained after expiry or release: a subsequent owner must advance the fence
// token, otherwise an old owner could accidentally reuse a token value.
func migration_021_add_execution_owner_leases(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS execution_owner_leases (
			account_id  TEXT PRIMARY KEY,
			owner_id    TEXT NOT NULL,
			fence_token INTEGER NOT NULL CHECK (fence_token > 0),
			expires_at  INTEGER NOT NULL,
			acquired_at INTEGER NOT NULL,
			renewed_at  INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS execution_writes (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id  TEXT NOT NULL,
			owner_id    TEXT NOT NULL,
			fence_token INTEGER NOT NULL CHECK (fence_token > 0),
			request_id  TEXT NOT NULL,
			write_kind  TEXT NOT NULL,
			payload     BLOB NOT NULL,
			created_at  INTEGER NOT NULL,
			UNIQUE (account_id, request_id)
		);
		CREATE INDEX IF NOT EXISTS idx_execution_writes_account
			ON execution_writes(account_id, id);
	`)
	return err
}
