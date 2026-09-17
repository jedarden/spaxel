// Package execution provides durable account-local ownership for execution
// engines and the fenced write primitive they use for execution state.
//
// An owner lease is not a process lock. A lease takeover advances the account's
// fence token, and every execution-class write must present both the owner and
// token that currently hold the account. This makes writes from an expired
// engine fail closed even if that engine is still running or recovers after a
// long pause.
package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// DefaultLeaseTTL is deliberately short enough that a stalled engine can be
	// replaced promptly while allowing normal heartbeat jitter.
	DefaultLeaseTTL = 30 * time.Second
)

var (
	// ErrLeaseHeld means another live owner currently holds the account lease.
	ErrLeaseHeld = errors.New("execution account lease is held")
	// ErrStaleFence means the caller no longer owns the account at the supplied
	// fence epoch, or that the lease has expired.
	ErrStaleFence = errors.New("stale execution fence")
	// ErrInvalidRequest is returned for missing identity or write fields.
	ErrInvalidRequest = errors.New("invalid execution lease request")
	// ErrRequestConflict means a request identity was already used for different
	// write contents.
	ErrRequestConflict = errors.New("execution request identity conflict")
)

// FenceToken is an account-local monotonically increasing fencing epoch. It is
// persisted as an SQLite INTEGER and is never reset when a lease is released.
type FenceToken int64

// Lease is the credential an execution engine must carry on all state writes.
type Lease struct {
	AccountID  string     `json:"account_id"`
	OwnerID    string     `json:"owner_id"`
	FenceToken FenceToken `json:"fence_token"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcquiredAt time.Time  `json:"acquired_at"`
	RenewedAt  time.Time  `json:"renewed_at"`
}

// ExecutionWrite is the complete envelope for an execution-class write. The
// fence token is a required field, not an optional metadata value.
type ExecutionWrite struct {
	AccountID  string     `json:"account_id"`
	OwnerID    string     `json:"owner_id"`
	FenceToken FenceToken `json:"fence_token"`
	RequestID  string     `json:"request_id"`
	Kind       string     `json:"kind"`
	Payload    []byte     `json:"payload"`
	CreatedAt  time.Time  `json:"created_at"`
}

// WriteReceipt identifies the durable write. Duplicate retries return the
// original ID with Duplicate=true after the current fence has been checked.
type WriteReceipt struct {
	ID        int64          `json:"id"`
	Write     ExecutionWrite `json:"write"`
	Duplicate bool           `json:"duplicate"`
}

type storedWrite struct {
	ID    int64
	Write ExecutionWrite
}

// Config controls Store timing. Clock is injected to make expiry and takeover
// behavior deterministic in tests; production uses time.Now.
type Config struct {
	LeaseTTL time.Duration
	Clock    func() time.Time
}

// Store persists account leases and execution-class writes in SQLite.
type Store struct {
	db    *sql.DB
	ttl   time.Duration
	clock func() time.Time
}

// NewStore creates an execution store and ensures its tables exist. Normal
// application startup also creates these tables through the schema migration;
// keeping this operation idempotent makes isolated callers and tests safe.
func NewStore(db *sql.DB) (*Store, error) {
	return NewStoreWithConfig(db, Config{})
}

// NewStoreWithConfig creates a Store with explicit lease timing.
func NewStoreWithConfig(db *sql.DB, cfg Config) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: nil database", ErrInvalidRequest)
	}
	ttl := cfg.LeaseTTL
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	// The mothership uses SQLite. One connection keeps in-memory test
	// databases coherent and matches the application's single-writer setup;
	// the ownership SQL itself remains atomic for file-backed databases too.
	db.SetMaxOpenConns(1)
	store := &Store{db: db, ttl: ttl, clock: clock}
	if err := store.EnsureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

// EnsureSchema creates the lease and fenced-write tables if they are absent.
// The statements match migration 021 and are safe to run repeatedly.
func (s *Store) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: nil store", ErrInvalidRequest)
	}
	_, err := s.db.ExecContext(ctx, `
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
	if err != nil {
		return fmt.Errorf("create execution lease schema: %w", err)
	}
	return nil
}

// Acquire obtains account ownership. A live different owner gets
// ErrLeaseHeld. The same owner may reacquire/heartbeat without changing its
// token; an expired lease, including one released by its previous owner,
// always advances the token.
func (s *Store) Acquire(ctx context.Context, accountID, ownerID string) (Lease, error) {
	if err := validateIdentity(accountID, ownerID); err != nil {
		return Lease{}, err
	}
	now := s.now()
	nowMS := now.UnixMilli()
	expiresMS := now.Add(s.ttl).UnixMilli()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO execution_owner_leases
			(account_id, owner_id, fence_token, expires_at, acquired_at, renewed_at)
		VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			owner_id = CASE
				WHEN execution_owner_leases.expires_at <= excluded.acquired_at
					OR execution_owner_leases.owner_id = excluded.owner_id
				THEN excluded.owner_id
				ELSE execution_owner_leases.owner_id
			END,
			fence_token = CASE
				WHEN execution_owner_leases.expires_at <= excluded.acquired_at
				THEN execution_owner_leases.fence_token + 1
				ELSE execution_owner_leases.fence_token
			END,
			expires_at = CASE
				WHEN execution_owner_leases.expires_at <= excluded.acquired_at
					OR execution_owner_leases.owner_id = excluded.owner_id
				THEN excluded.expires_at
				ELSE execution_owner_leases.expires_at
			END,
			acquired_at = CASE
				WHEN execution_owner_leases.expires_at <= excluded.acquired_at
				THEN excluded.acquired_at
				ELSE execution_owner_leases.acquired_at
			END,
			renewed_at = CASE
				WHEN execution_owner_leases.expires_at <= excluded.acquired_at
					OR execution_owner_leases.owner_id = excluded.owner_id
				THEN excluded.renewed_at
				ELSE execution_owner_leases.renewed_at
			END
	`, accountID, ownerID, expiresMS, nowMS, nowMS)
	if err != nil {
		return Lease{}, fmt.Errorf("acquire execution lease: %w", err)
	}

	lease, err := s.loadLease(ctx, accountID)
	if err != nil {
		return Lease{}, err
	}
	if lease.OwnerID != ownerID || !lease.ExpiresAt.After(now) {
		return Lease{}, fmt.Errorf("%w: account %q is owned by %q", ErrLeaseHeld, accountID, lease.OwnerID)
	}
	return lease, nil
}

// Renew extends a lease only when its account, owner, and fence token still
// match an unexpired row. An expired or superseded engine cannot revive itself.
func (s *Store) Renew(ctx context.Context, lease Lease) (Lease, error) {
	if err := validateLease(lease); err != nil {
		return Lease{}, err
	}
	now := s.now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE execution_owner_leases
		SET expires_at = ?, renewed_at = ?
		WHERE account_id = ? AND owner_id = ? AND fence_token = ? AND expires_at > ?
	`, now.Add(s.ttl).UnixMilli(), now.UnixMilli(), lease.AccountID, lease.OwnerID,
		int64(lease.FenceToken), now.UnixMilli())
	if err != nil {
		return Lease{}, fmt.Errorf("renew execution lease: %w", err)
	}
	current, err := s.loadLease(ctx, lease.AccountID)
	if err != nil {
		return Lease{}, err
	}
	if current.OwnerID != lease.OwnerID || current.FenceToken != lease.FenceToken ||
		!current.ExpiresAt.After(now) {
		return Lease{}, ErrStaleFence
	}
	return current, nil
}

// Release expires a lease without deleting its row, preserving the fence
// history so a future acquisition cannot reuse the old token.
func (s *Store) Release(ctx context.Context, lease Lease) error {
	if err := validateLease(lease); err != nil {
		return err
	}
	nowMS := s.now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		UPDATE execution_owner_leases
		SET owner_id = '', expires_at = ?, renewed_at = ?
		WHERE account_id = ? AND owner_id = ? AND fence_token = ? AND expires_at > ?
	`, nowMS, nowMS, lease.AccountID, lease.OwnerID, int64(lease.FenceToken), nowMS)
	if err != nil {
		return fmt.Errorf("release execution lease: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil {
		return fmt.Errorf("release execution lease rows affected: %w", affectedErr)
	} else if affected != 1 {
		return ErrStaleFence
	}
	return nil
}

// Current returns the active lease for an account. Expired and released rows
// are treated as absent while remaining in the database for fence continuity.
func (s *Store) Current(ctx context.Context, accountID string) (Lease, bool, error) {
	if accountID == "" {
		return Lease{}, false, fmt.Errorf("%w: empty account ID", ErrInvalidRequest)
	}
	lease, err := s.loadLease(ctx, accountID)
	if err == sql.ErrNoRows {
		return Lease{}, false, nil
	}
	if err != nil {
		return Lease{}, false, err
	}
	if lease.OwnerID == "" || !lease.ExpiresAt.After(s.now()) {
		return Lease{}, false, nil
	}
	return lease, true, nil
}

// Append durably records one execution-class write only if the supplied
// account/owner/fence tuple is current at the same SQLite write boundary as
// the insert. This closes the check-then-write race with lease takeover.
func (s *Store) Append(ctx context.Context, write ExecutionWrite) (WriteReceipt, error) {
	if err := validateWrite(write); err != nil {
		return WriteReceipt{}, err
	}
	now := s.now()
	createdAtProvided := !write.CreatedAt.IsZero()
	if write.CreatedAt.IsZero() {
		write.CreatedAt = now
	}
	if write.Payload == nil {
		write.Payload = []byte{}
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO execution_writes
			(account_id, owner_id, fence_token, request_id, write_kind, payload, created_at)
		SELECT ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM execution_owner_leases
			WHERE account_id = ? AND owner_id = ? AND fence_token = ? AND expires_at > ?
		)
	`, write.AccountID, write.OwnerID, int64(write.FenceToken), write.RequestID,
		write.Kind, write.Payload, write.CreatedAt.UnixMilli(), write.AccountID,
		write.OwnerID, int64(write.FenceToken), now.UnixMilli())
	if err != nil {
		return WriteReceipt{}, fmt.Errorf("append execution write: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return WriteReceipt{}, fmt.Errorf("append execution write rows affected: %w", err)
	}
	if affected == 1 {
		stored, found, loadErr := s.loadWrite(ctx, write.AccountID, write.RequestID)
		if loadErr != nil {
			return WriteReceipt{}, loadErr
		}
		if !found {
			return WriteReceipt{}, fmt.Errorf("execution write inserted but could not be read")
		}
		return WriteReceipt{ID: stored.ID, Write: stored.Write}, nil
	}

	// Check the fence before interpreting a zero-row insert as an idempotent
	// retry. A stale caller must not be able to observe success merely because a
	// request ID was used by an earlier lease epoch.
	current, currentErr := s.loadLease(ctx, write.AccountID)
	if currentErr != nil {
		return WriteReceipt{}, currentErr
	}
	if current.OwnerID != write.OwnerID || current.FenceToken != write.FenceToken ||
		!current.ExpiresAt.After(s.now()) {
		return WriteReceipt{}, ErrStaleFence
	}

	stored, found, err := s.loadWrite(ctx, write.AccountID, write.RequestID)
	if err != nil {
		return WriteReceipt{}, err
	}
	if !found {
		return WriteReceipt{}, ErrStaleFence
	}
	if !sameWrite(stored.Write, write, createdAtProvided) {
		return WriteReceipt{}, ErrRequestConflict
	}
	return WriteReceipt{ID: stored.ID, Write: stored.Write, Duplicate: true}, nil
}

// AcquireLease is an explicit alias for callers that prefer operation names
// in their engine code.
func (s *Store) AcquireLease(ctx context.Context, accountID, ownerID string) (Lease, error) {
	return s.Acquire(ctx, accountID, ownerID)
}

// RenewLease is an explicit alias for callers that prefer operation names in
// their engine code.
func (s *Store) RenewLease(ctx context.Context, lease Lease) (Lease, error) {
	return s.Renew(ctx, lease)
}

// ReleaseLease is an explicit alias for callers that prefer operation names
// in their engine code.
func (s *Store) ReleaseLease(ctx context.Context, lease Lease) error {
	return s.Release(ctx, lease)
}

// AppendExecutionWrite is an explicit alias that makes the execution-class
// boundary visible at call sites.
func (s *Store) AppendExecutionWrite(ctx context.Context, write ExecutionWrite) (WriteReceipt, error) {
	return s.Append(ctx, write)
}

func (s *Store) now() time.Time {
	return s.clock().UTC()
}

func (s *Store) loadLease(ctx context.Context, accountID string) (Lease, error) {
	var (
		ownerID                          string
		fenceToken                       int64
		expiresAt, acquiredAt, renewedAt int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_id, fence_token, expires_at, acquired_at, renewed_at
		FROM execution_owner_leases WHERE account_id = ?
	`, accountID).Scan(&ownerID, &fenceToken, &expiresAt, &acquiredAt, &renewedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return Lease{}, sql.ErrNoRows
		}
		return Lease{}, fmt.Errorf("load execution lease: %w", err)
	}
	return Lease{
		AccountID:  accountID,
		OwnerID:    ownerID,
		FenceToken: FenceToken(fenceToken),
		ExpiresAt:  time.UnixMilli(expiresAt).UTC(),
		AcquiredAt: time.UnixMilli(acquiredAt).UTC(),
		RenewedAt:  time.UnixMilli(renewedAt).UTC(),
	}, nil
}

func (s *Store) loadWrite(ctx context.Context, accountID, requestID string) (storedWrite, bool, error) {
	var (
		id         int64
		ownerID    string
		fenceToken int64
		kind       string
		payload    []byte
		createdAt  int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, fence_token, write_kind, payload, created_at
		FROM execution_writes WHERE account_id = ? AND request_id = ?
	`, accountID, requestID).Scan(&id, &ownerID, &fenceToken, &kind, &payload, &createdAt)
	if err == sql.ErrNoRows {
		return storedWrite{}, false, nil
	}
	if err != nil {
		return storedWrite{}, false, fmt.Errorf("load execution write: %w", err)
	}
	return storedWrite{
		ID: id,
		Write: ExecutionWrite{
			AccountID:  accountID,
			OwnerID:    ownerID,
			FenceToken: FenceToken(fenceToken),
			RequestID:  requestID,
			Kind:       kind,
			Payload:    append([]byte(nil), payload...),
			CreatedAt:  time.UnixMilli(createdAt).UTC(),
		},
	}, true, nil
}

func sameWrite(a, b ExecutionWrite, compareCreatedAt bool) bool {
	// OwnerID and FenceToken authorize the retry at the current lease boundary;
	// they are not request content. A replacement owner must be able to resume
	// an already-recorded request without creating a second durable row. The
	// caller's current fence is checked before this comparison, so an expired
	// owner cannot use the same request ID to obtain a duplicate receipt.
	return a.AccountID == b.AccountID && a.RequestID == b.RequestID &&
		a.Kind == b.Kind && string(a.Payload) == string(b.Payload) &&
		(!compareCreatedAt || a.CreatedAt.UnixMilli() == b.CreatedAt.UnixMilli())
}

func validateIdentity(accountID, ownerID string) error {
	if accountID == "" || ownerID == "" {
		return fmt.Errorf("%w: account and owner IDs are required", ErrInvalidRequest)
	}
	return nil
}

func validateLease(lease Lease) error {
	if err := validateIdentity(lease.AccountID, lease.OwnerID); err != nil {
		return err
	}
	if lease.FenceToken <= 0 {
		return fmt.Errorf("%w: fence token must be positive", ErrInvalidRequest)
	}
	return nil
}

func validateWrite(write ExecutionWrite) error {
	if err := validateIdentity(write.AccountID, write.OwnerID); err != nil {
		return err
	}
	if write.FenceToken <= 0 {
		return fmt.Errorf("%w: fence token must be positive", ErrInvalidRequest)
	}
	if write.RequestID == "" || write.Kind == "" {
		return fmt.Errorf("%w: request ID and write kind are required", ErrInvalidRequest)
	}
	return nil
}
