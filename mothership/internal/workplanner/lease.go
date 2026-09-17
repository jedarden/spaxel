// Package workplanner provides durable ownership for the mothership's work
// planner and a resumable work-item queue.
//
// The planner lease is a database lease, not a process lock. Taking over an
// expired lease advances its fence token. Every planner write carries the
// owner and token and is checked at the same SQLite write boundary as the
// write itself, so an instance that pauses or loses its process cannot write
// after a replacement has taken over.
package workplanner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// DefaultLeaseTTL allows a replacement to take over promptly while leaving
	// enough room for normal scheduler and SQLite latency.
	DefaultLeaseTTL = 30 * time.Second
	// DefaultClaimTTL is the worker lease duration for an individual item.
	DefaultClaimTTL  = 2 * time.Minute
	defaultPlannerID = "default"
)

var (
	// ErrLeaseHeld means another live planner instance owns the planner lease.
	ErrLeaseHeld = errors.New("work planner lease is held")
	// ErrStaleFence means the planner no longer owns the supplied fence epoch.
	ErrStaleFence = errors.New("stale work planner fence")
	// ErrStaleClaim means a worker lost an item claim, normally because it
	// stopped heartbeating and another worker reclaimed the item.
	ErrStaleClaim = errors.New("stale work item claim")
	// ErrInvalidRequest identifies missing or invalid durable work fields.
	ErrInvalidRequest = errors.New("invalid work planner request")
	// ErrRequestConflict means an idempotency identity was reused for different
	// work contents.
	ErrRequestConflict = errors.New("work item identity conflict")
	// ErrNoWork means no queued or expired item is currently eligible.
	ErrNoWork = errors.New("no work available")
)

// FenceToken is a planner-instance fencing epoch. It is never reset when the
// lease is released or expires.
type FenceToken int64

// Lease is the durable credential a planner carries on every planner write.
type Lease struct {
	PlannerID  string     `json:"planner_id"`
	OwnerID    string     `json:"owner_id"`
	FenceToken FenceToken `json:"fence_token"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcquiredAt time.Time  `json:"acquired_at"`
	RenewedAt  time.Time  `json:"renewed_at"`
}

// WorkItem is a durable unit of helper work. ID and DedupeKey are both
// required identities: ID addresses the item, while DedupeKey prevents a
// planner retry from creating a second logical item under a new ID.
type WorkItem struct {
	ID           string    `json:"id"`
	DedupeKey    string    `json:"dedupe_key"`
	Kind         string    `json:"kind"`
	Payload      []byte    `json:"payload"`
	AvailableAt  time.Time `json:"available_at"`
	AttemptLimit int       `json:"attempt_limit"`
}

// WorkItemState describes the durable lifecycle of an item.
type WorkItemState string

const (
	StateQueued    WorkItemState = "queued"
	StateLeased    WorkItemState = "leased"
	StateCompleted WorkItemState = "completed"
	StateFailed    WorkItemState = "failed"
)

// WorkItemRecord is the persisted item returned by Get and claim operations.
type WorkItemRecord struct {
	WorkItem
	State          WorkItemState `json:"state"`
	AttemptCount   int           `json:"attempt_count"`
	ClaimOwnerID   string        `json:"claim_owner_id"`
	ClaimToken     int64         `json:"claim_token"`
	ClaimExpiresAt time.Time     `json:"claim_expires_at"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	CompletedAt    time.Time     `json:"completed_at"`
	LastError      string        `json:"last_error"`
}

// WorkReceipt describes an enqueue result. Duplicate retries return the
// original item with Duplicate=true.
type WorkReceipt struct {
	Item      WorkItemRecord `json:"item"`
	Duplicate bool           `json:"duplicate"`
}

// Claim is the worker's fencing credential for an item. ClaimToken changes
// on every reclaim, so an old worker cannot complete a replacement's claim.
type Claim struct {
	ItemID     string    `json:"item_id"`
	WorkerID   string    `json:"worker_id"`
	ClaimToken int64     `json:"claim_token"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Config controls planner and worker lease timing.
type Config struct {
	PlannerID string
	LeaseTTL  time.Duration
	ClaimTTL  time.Duration
	Clock     func() time.Time
}

// Store persists planner leases and work items in SQLite.
type Store struct {
	db        *sql.DB
	plannerID string
	leaseTTL  time.Duration
	claimTTL  time.Duration
	clock     func() time.Time
}

// NewStore creates a default planner store and ensures its tables exist.
func NewStore(db *sql.DB) (*Store, error) {
	return NewStoreWithConfig(db, Config{})
}

// NewStoreWithConfig creates a planner store with deterministic timing when a
// clock is supplied. Production callers should use the default wall clock.
func NewStoreWithConfig(db *sql.DB, cfg Config) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: nil database", ErrInvalidRequest)
	}
	plannerID := cfg.PlannerID
	if plannerID == "" {
		plannerID = defaultPlannerID
	}
	leaseTTL := cfg.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = DefaultLeaseTTL
	}
	claimTTL := cfg.ClaimTTL
	if claimTTL <= 0 {
		claimTTL = DefaultClaimTTL
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	db.SetMaxOpenConns(1)
	store := &Store{
		db:        db,
		plannerID: plannerID,
		leaseTTL:  leaseTTL,
		claimTTL:  claimTTL,
		clock:     clock,
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

// EnsureSchema is idempotent and matches migration 022. It exists so the
// isolated store can be used by tests and small callers before the application
// migration runner is wired into a process.
func (s *Store) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: nil store", ErrInvalidRequest)
	}
	_, err := s.db.ExecContext(ctx, `
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
	if err != nil {
		return fmt.Errorf("create work planner schema: %w", err)
	}
	return nil
}

// Acquire obtains the planner lease. A live different owner is rejected; the
// same owner heartbeats without changing its fence; a takeover advances it.
func (s *Store) Acquire(ctx context.Context, ownerID string) (Lease, error) {
	if ownerID == "" {
		return Lease{}, fmt.Errorf("%w: owner ID is required", ErrInvalidRequest)
	}
	now := s.now()
	nowMS := now.UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO work_planner_leases
			(planner_id, owner_id, fence_token, expires_at, acquired_at, renewed_at)
		VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT(planner_id) DO UPDATE SET
			owner_id = CASE
				WHEN work_planner_leases.expires_at <= excluded.acquired_at
					OR work_planner_leases.owner_id = excluded.owner_id
				THEN excluded.owner_id
				ELSE work_planner_leases.owner_id
			END,
			fence_token = CASE
				WHEN work_planner_leases.expires_at <= excluded.acquired_at
				THEN work_planner_leases.fence_token + 1
				ELSE work_planner_leases.fence_token
			END,
			expires_at = CASE
				WHEN work_planner_leases.expires_at <= excluded.acquired_at
					OR work_planner_leases.owner_id = excluded.owner_id
				THEN excluded.expires_at
				ELSE work_planner_leases.expires_at
			END,
			acquired_at = CASE
				WHEN work_planner_leases.expires_at <= excluded.acquired_at
				THEN excluded.acquired_at
				ELSE work_planner_leases.acquired_at
			END,
			renewed_at = CASE
				WHEN work_planner_leases.expires_at <= excluded.acquired_at
					OR work_planner_leases.owner_id = excluded.owner_id
				THEN excluded.renewed_at
				ELSE work_planner_leases.renewed_at
			END
	`, s.plannerID, ownerID, now.Add(s.leaseTTL).UnixMilli(), nowMS, nowMS)
	if err != nil {
		return Lease{}, fmt.Errorf("acquire work planner lease: %w", err)
	}
	lease, err := s.loadLease(ctx)
	if err != nil {
		return Lease{}, err
	}
	if lease.OwnerID != ownerID || !lease.ExpiresAt.After(now) {
		return Lease{}, fmt.Errorf("%w: planner %q is owned by %q", ErrLeaseHeld, s.plannerID, lease.OwnerID)
	}
	return lease, nil
}

// Renew extends a lease only when its owner and fence still match an
// unexpired row. An old instance cannot revive itself after takeover.
func (s *Store) Renew(ctx context.Context, lease Lease) (Lease, error) {
	if err := s.validateLease(lease); err != nil {
		return Lease{}, err
	}
	now := s.now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE work_planner_leases
		SET expires_at = ?, renewed_at = ?
		WHERE planner_id = ? AND owner_id = ? AND fence_token = ? AND expires_at > ?
	`, now.Add(s.leaseTTL).UnixMilli(), now.UnixMilli(), s.plannerID, lease.OwnerID,
		int64(lease.FenceToken), now.UnixMilli())
	if err != nil {
		return Lease{}, fmt.Errorf("renew work planner lease: %w", err)
	}
	current, err := s.loadLease(ctx)
	if err != nil {
		return Lease{}, err
	}
	if current.OwnerID != lease.OwnerID || current.FenceToken != lease.FenceToken ||
		!current.ExpiresAt.After(now) {
		return Lease{}, ErrStaleFence
	}
	return current, nil
}

// Release expires the current lease without deleting its fence history.
func (s *Store) Release(ctx context.Context, lease Lease) error {
	if err := s.validateLease(lease); err != nil {
		return err
	}
	nowMS := s.now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		UPDATE work_planner_leases
		SET owner_id = '', expires_at = ?, renewed_at = ?
		WHERE planner_id = ? AND owner_id = ? AND fence_token = ? AND expires_at > ?
	`, nowMS, nowMS, s.plannerID, lease.OwnerID, int64(lease.FenceToken), nowMS)
	if err != nil {
		return fmt.Errorf("release work planner lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("release work planner lease rows affected: %w", err)
	}
	if affected != 1 {
		return ErrStaleFence
	}
	return nil
}

// Current returns the active planner lease, if any. Expired rows remain in
// SQLite so their fencing epoch cannot be reused.
func (s *Store) Current(ctx context.Context) (Lease, bool, error) {
	lease, err := s.loadLease(ctx)
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

// Enqueue durably schedules an item only while lease is current. The fence is
// checked inside the INSERT, closing the takeover race between an external
// lease check and the write. Retries with the same ID and content are safe.
func (s *Store) Enqueue(ctx context.Context, lease Lease, item WorkItem) (WorkReceipt, error) {
	if err := s.validateLease(lease); err != nil {
		return WorkReceipt{}, err
	}
	if err := validateWorkItem(item); err != nil {
		return WorkReceipt{}, err
	}
	now := s.now()
	availableWasProvided := !item.AvailableAt.IsZero()
	availableAt := item.AvailableAt
	if availableAt.IsZero() {
		availableAt = now
	}
	if item.Payload == nil {
		item.Payload = []byte{}
	}
	attemptLimit := item.AttemptLimit
	if attemptLimit <= 0 {
		attemptLimit = 3
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO work_items
			(id, dedupe_key, work_kind, payload, state, available_at,
			 attempt_limit, created_at, updated_at)
		SELECT ?, ?, ?, ?, 'queued', ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM work_planner_leases
			WHERE planner_id = ? AND owner_id = ? AND fence_token = ? AND expires_at > ?
		)
	`, item.ID, item.DedupeKey, item.Kind, item.Payload, availableAt.UnixMilli(),
		attemptLimit, now.UnixMilli(), now.UnixMilli(), s.plannerID, lease.OwnerID,
		int64(lease.FenceToken), now.UnixMilli())
	if err != nil {
		return WorkReceipt{}, fmt.Errorf("enqueue work item: %w", err)
	}

	record, found, err := s.loadItem(ctx, item.ID)
	if err != nil {
		return WorkReceipt{}, err
	}
	if found {
		current, currentErr := s.loadLease(ctx)
		if currentErr != nil {
			return WorkReceipt{}, currentErr
		}
		if current.OwnerID != lease.OwnerID || current.FenceToken != lease.FenceToken ||
			!current.ExpiresAt.After(s.now()) {
			return WorkReceipt{}, ErrStaleFence
		}
		if !sameWork(record, item, availableAt, attemptLimit, availableWasProvided) {
			return WorkReceipt{}, ErrRequestConflict
		}
		return WorkReceipt{Item: record, Duplicate: true}, nil
	}
	// A different item ID using the same dedupe key is a conflict, not a
	// missing/stale lease. Check it only after the fence so an expired planner
	// cannot use the lookup to learn that its rejected write succeeded.
	_, existingFound, err := s.loadItemByDedupe(ctx, item.DedupeKey)
	if err != nil {
		return WorkReceipt{}, err
	}
	if existingFound {
		current, currentErr := s.loadLease(ctx)
		if currentErr != nil {
			return WorkReceipt{}, currentErr
		}
		if current.OwnerID != lease.OwnerID || current.FenceToken != lease.FenceToken ||
			!current.ExpiresAt.After(s.now()) {
			return WorkReceipt{}, ErrStaleFence
		}
		return WorkReceipt{}, ErrRequestConflict
	}
	return WorkReceipt{}, ErrStaleFence
}

// Schedule is an alias that emphasizes the planner-facing operation.
func (s *Store) Schedule(ctx context.Context, lease Lease, item WorkItem) (WorkReceipt, error) {
	return s.Enqueue(ctx, lease, item)
}

// Get returns an item by stable ID, including a completed or failed item.
func (s *Store) Get(ctx context.Context, itemID string) (WorkItemRecord, bool, error) {
	if itemID == "" {
		return WorkItemRecord{}, false, fmt.Errorf("%w: item ID is required", ErrInvalidRequest)
	}
	return s.loadItem(ctx, itemID)
}

// ClaimNext atomically claims the highest-priority eligible item for a worker.
// WorkItem currently has no public priority field; insertion order (available
// time, then ID) is deterministic until priority becomes a domain contract.
// Expired claims are eligible for takeover.
func (s *Store) ClaimNext(ctx context.Context, workerID string) (Claim, WorkItemRecord, error) {
	if workerID == "" {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("%w: worker ID is required", ErrInvalidRequest)
	}
	now := s.now()
	nowMS := now.UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("begin work claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE work_items
		SET state = 'failed', claim_owner_id = '', claim_expires_at = 0,
			last_error = 'claim expired after attempt limit', updated_at = ?
		WHERE state = 'leased' AND claim_expires_at <= ? AND attempt_count >= attempt_limit
	`, nowMS, nowMS); err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("expire exhausted work item: %w", err)
	}

	var itemID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM work_items
		WHERE (state = 'queued' AND available_at <= ?)
		   OR (state = 'leased' AND claim_expires_at <= ? AND attempt_count < attempt_limit)
		ORDER BY available_at ASC, id ASC
		LIMIT 1
	`, nowMS, nowMS).Scan(&itemID)
	if err == sql.ErrNoRows {
		return Claim{}, WorkItemRecord{}, ErrNoWork
	}
	if err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("find work item: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE work_items
		SET state = 'leased', claim_owner_id = ?, claim_token = claim_token + 1,
			claim_expires_at = ?, attempt_count = attempt_count + 1, updated_at = ?
		WHERE id = ? AND ((state = 'queued' AND available_at <= ?) OR
			(state = 'leased' AND claim_expires_at <= ?))
	`, workerID, now.Add(s.claimTTL).UnixMilli(), nowMS, itemID, nowMS, nowMS)
	if err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("claim work item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("claim work item rows affected: %w", err)
	}
	if affected != 1 {
		return Claim{}, WorkItemRecord{}, ErrNoWork
	}
	var token int64
	if err := tx.QueryRowContext(ctx, `SELECT claim_token FROM work_items WHERE id = ?`, itemID).Scan(&token); err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("load claimed token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("commit work claim: %w", err)
	}
	record, found, err := s.loadItem(ctx, itemID)
	if err != nil {
		return Claim{}, WorkItemRecord{}, err
	}
	if !found {
		return Claim{}, WorkItemRecord{}, fmt.Errorf("claimed work item %q disappeared", itemID)
	}
	return Claim{ItemID: itemID, WorkerID: workerID, ClaimToken: token, ExpiresAt: record.ClaimExpiresAt}, record, nil
}

// Heartbeat extends a worker claim only while its token is current and
// unexpired.
func (s *Store) Heartbeat(ctx context.Context, claim Claim) (Claim, error) {
	if err := validateClaim(claim); err != nil {
		return Claim{}, err
	}
	now := s.now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE work_items
		SET claim_expires_at = ?, updated_at = ?
		WHERE id = ? AND state = 'leased' AND claim_owner_id = ? AND
			claim_token = ? AND claim_expires_at > ?
	`, now.Add(s.claimTTL).UnixMilli(), now.UnixMilli(), claim.ItemID, claim.WorkerID,
		claim.ClaimToken, now.UnixMilli())
	if err != nil {
		return Claim{}, fmt.Errorf("heartbeat work claim: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Claim{}, fmt.Errorf("heartbeat work claim rows affected: %w", err)
	}
	if affected != 1 {
		return Claim{}, ErrStaleClaim
	}
	claim.ExpiresAt = now.Add(s.claimTTL)
	return claim, nil
}

// Complete marks a claimed item complete only for the current worker token.
// A replacement worker can safely complete a reclaimed item with its new
// token; the old worker cannot.
func (s *Store) Complete(ctx context.Context, claim Claim) error {
	if err := validateClaim(claim); err != nil {
		return err
	}
	now := s.now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE work_items
		SET state = 'completed', claim_owner_id = '', claim_expires_at = 0,
			completed_at = ?, updated_at = ?
		WHERE id = ? AND state = 'leased' AND claim_owner_id = ? AND
			claim_token = ? AND claim_expires_at > ?
	`, nowMS(now), nowMS(now), claim.ItemID, claim.WorkerID, claim.ClaimToken, nowMS(now))
	if err != nil {
		return fmt.Errorf("complete work item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("complete work item rows affected: %w", err)
	}
	if affected != 1 {
		return ErrStaleClaim
	}
	return nil
}

// Fail records an error and either requeues the item for a retry or marks it
// failed when its attempt limit is exhausted. It is fenced by the claim.
func (s *Store) Fail(ctx context.Context, claim Claim, reason string, retryAt time.Time) error {
	if err := validateClaim(claim); err != nil {
		return err
	}
	if reason == "" {
		return fmt.Errorf("%w: failure reason is required", ErrInvalidRequest)
	}
	now := s.now()
	availableAt := retryAt
	if availableAt.IsZero() {
		availableAt = now
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE work_items
		SET state = CASE WHEN attempt_count < attempt_limit THEN 'queued' ELSE 'failed' END,
			available_at = ?, claim_owner_id = '', claim_expires_at = 0,
			last_error = ?, updated_at = ?
		WHERE id = ? AND state = 'leased' AND claim_owner_id = ? AND
			claim_token = ? AND claim_expires_at > ?
	`, availableAt.UnixMilli(), reason, now.UnixMilli(), claim.ItemID, claim.WorkerID,
		claim.ClaimToken, now.UnixMilli())
	if err != nil {
		return fmt.Errorf("fail work item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("fail work item rows affected: %w", err)
	}
	if affected != 1 {
		return ErrStaleClaim
	}
	return nil
}

// AcquireLease, RenewLease, and ReleaseLease are explicit aliases for callers
// that prefer operation names at their planner boundary.
func (s *Store) AcquireLease(ctx context.Context, ownerID string) (Lease, error) {
	return s.Acquire(ctx, ownerID)
}

func (s *Store) RenewLease(ctx context.Context, lease Lease) (Lease, error) {
	return s.Renew(ctx, lease)
}

func (s *Store) ReleaseLease(ctx context.Context, lease Lease) error {
	return s.Release(ctx, lease)
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) loadLease(ctx context.Context) (Lease, error) {
	var owner string
	var token, expires, acquired, renewed int64
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_id, fence_token, expires_at, acquired_at, renewed_at
		FROM work_planner_leases WHERE planner_id = ?
	`, s.plannerID).Scan(&owner, &token, &expires, &acquired, &renewed)
	if err != nil {
		return Lease{}, err
	}
	return Lease{
		PlannerID:  s.plannerID,
		OwnerID:    owner,
		FenceToken: FenceToken(token),
		ExpiresAt:  time.UnixMilli(expires).UTC(),
		AcquiredAt: time.UnixMilli(acquired).UTC(),
		RenewedAt:  time.UnixMilli(renewed).UTC(),
	}, nil
}

func (s *Store) loadItem(ctx context.Context, itemID string) (WorkItemRecord, bool, error) {
	var (
		id, dedupe, kind, state, owner, payload, lastError     string
		available, attempts, attemptLimit, token, claimExpires int64
		created, updated, completed                            sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, dedupe_key, work_kind, payload, state, available_at,
			attempt_count, attempt_limit, claim_owner_id, claim_token,
			claim_expires_at, created_at, updated_at, completed_at, last_error
		FROM work_items WHERE id = ?
	`, itemID).Scan(&id, &dedupe, &kind, &payload, &state, &available, &attempts,
		&attemptLimit, &owner, &token, &claimExpires, &created, &updated, &completed, &lastError)
	if err == sql.ErrNoRows {
		return WorkItemRecord{}, false, nil
	}
	if err != nil {
		return WorkItemRecord{}, false, fmt.Errorf("load work item: %w", err)
	}
	record := WorkItemRecord{
		WorkItem: WorkItem{ID: id, DedupeKey: dedupe, Kind: kind, Payload: append([]byte(nil), payload...),
			AvailableAt: time.UnixMilli(available).UTC(), AttemptLimit: int(attemptLimit)},
		State:          WorkItemState(state),
		AttemptCount:   int(attempts),
		ClaimOwnerID:   owner,
		ClaimToken:     token,
		ClaimExpiresAt: time.UnixMilli(claimExpires).UTC(),
		CreatedAt:      time.UnixMilli(created.Int64).UTC(),
		UpdatedAt:      time.UnixMilli(updated.Int64).UTC(),
		LastError:      lastError,
	}
	if completed.Valid {
		record.CompletedAt = time.UnixMilli(completed.Int64).UTC()
	}
	return record, true, nil
}

func (s *Store) loadItemByDedupe(ctx context.Context, dedupeKey string) (WorkItemRecord, bool, error) {
	var itemID string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM work_items WHERE dedupe_key = ?
	`, dedupeKey).Scan(&itemID)
	if err == sql.ErrNoRows {
		return WorkItemRecord{}, false, nil
	}
	if err != nil {
		return WorkItemRecord{}, false, fmt.Errorf("find work item by dedupe key: %w", err)
	}
	return s.loadItem(ctx, itemID)
}

func (s *Store) validateLease(lease Lease) error {
	if lease.PlannerID != "" && lease.PlannerID != s.plannerID {
		return fmt.Errorf("%w: lease belongs to planner %q", ErrInvalidRequest, lease.PlannerID)
	}
	if lease.OwnerID == "" || lease.FenceToken <= 0 {
		return fmt.Errorf("%w: owner and positive fence token are required", ErrInvalidRequest)
	}
	return nil
}

func validateWorkItem(item WorkItem) error {
	if item.ID == "" || item.DedupeKey == "" || item.Kind == "" {
		return fmt.Errorf("%w: item ID, dedupe key, and kind are required", ErrInvalidRequest)
	}
	if item.AttemptLimit < 0 {
		return fmt.Errorf("%w: attempt limit cannot be negative", ErrInvalidRequest)
	}
	return nil
}

func validateClaim(claim Claim) error {
	if claim.ItemID == "" || claim.WorkerID == "" || claim.ClaimToken <= 0 {
		return fmt.Errorf("%w: item, worker, and positive claim token are required", ErrInvalidRequest)
	}
	return nil
}

func sameWork(record WorkItemRecord, item WorkItem, availableAt time.Time, attemptLimit int, compareAvailableAt bool) bool {
	return record.ID == item.ID && record.DedupeKey == item.DedupeKey && record.Kind == item.Kind &&
		string(record.Payload) == string(item.Payload) &&
		(!compareAvailableAt || record.AvailableAt.UnixMilli() == availableAt.UnixMilli()) &&
		record.AttemptLimit == attemptLimit
}

func nowMS(t time.Time) int64 { return t.UnixMilli() }
