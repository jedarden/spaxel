package execution

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func newTestStore(t *testing.T, ttl time.Duration) (*Store, *testClock, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "execution.db"))+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	clock := &testClock{now: time.UnixMilli(1_000_000).UTC()}
	store, err := NewStoreWithConfig(db, Config{LeaseTTL: ttl, Clock: clock.Now})
	if err != nil {
		t.Fatalf("new execution store: %v", err)
	}
	return store, clock, db
}

func TestAcquireLeaseTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		ownerID   string
		wantErr   error
	}{
		{name: "missing account", ownerID: "engine-a", wantErr: ErrInvalidRequest},
		{name: "missing owner", accountID: "account-a", wantErr: ErrInvalidRequest},
		{name: "valid", accountID: "account-a", ownerID: "engine-a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _, _ := newTestStore(t, time.Minute)
			lease, err := store.Acquire(context.Background(), tt.accountID, tt.ownerID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Acquire error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			if lease.FenceToken != 1 {
				t.Fatalf("first fence token = %d, want 1", lease.FenceToken)
			}
		})
	}
}

func TestAcquireRejectsSecondLiveOwner(t *testing.T) {
	store, clock, db := newTestStore(t, time.Minute)
	ctx := context.Background()

	first, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	second, err := store.Acquire(ctx, "account-a", "engine-b")
	if !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("second Acquire error = %v, want ErrLeaseHeld", err)
	}
	if second != (Lease{}) {
		t.Fatalf("failed Acquire returned lease %#v", second)
	}

	current, ok, err := store.Current(ctx, "account-a")
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !ok || current.OwnerID != first.OwnerID || current.FenceToken != first.FenceToken {
		t.Fatalf("Current = %#v, active = %v, want first lease %#v", current, ok, first)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_owner_leases WHERE account_id = 'account-a'`).Scan(&count); err != nil {
		t.Fatalf("count lease rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("lease row count = %d, want 1", count)
	}

	clock.Set(clock.Now().Add(2 * time.Minute))
	third, err := store.Acquire(ctx, "account-a", "engine-b")
	if err != nil {
		t.Fatalf("expired takeover Acquire: %v", err)
	}
	if third.OwnerID != "engine-b" || third.FenceToken != first.FenceToken+1 {
		t.Fatalf("takeover lease = %#v, want owner engine-b and token %d", third, first.FenceToken+1)
	}
}

func TestSameOwnerHeartbeatKeepsFence(t *testing.T) {
	store, clock, _ := newTestStore(t, time.Minute)
	ctx := context.Background()

	first, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	clock.Set(clock.Now().Add(10 * time.Second))
	second, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("same-owner Acquire: %v", err)
	}
	if second.FenceToken != first.FenceToken {
		t.Fatalf("same-owner token = %d, want unchanged %d", second.FenceToken, first.FenceToken)
	}
	if !second.ExpiresAt.After(first.ExpiresAt) {
		t.Fatalf("same-owner expiry = %s, want after %s", second.ExpiresAt, first.ExpiresAt)
	}
}

func TestReleaseAdvancesFenceOnNextAcquire(t *testing.T) {
	store, _, _ := newTestStore(t, time.Minute)
	ctx := context.Background()

	first, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if err := store.Release(ctx, first); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, ok, err := store.Current(ctx, "account-a"); err != nil {
		t.Fatalf("Current after Release: %v", err)
	} else if ok {
		t.Fatal("released lease is still active")
	}

	second, err := store.Acquire(ctx, "account-a", "engine-b")
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if second.FenceToken != first.FenceToken+1 {
		t.Fatalf("second token = %d, want %d", second.FenceToken, first.FenceToken+1)
	}
	if err := store.Release(ctx, first); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale Release error = %v, want ErrStaleFence", err)
	}
}

func TestFencedExecutionWriteRejectsStaleOwner(t *testing.T) {
	store, clock, db := newTestStore(t, time.Minute)
	ctx := context.Background()

	first, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	firstWrite := ExecutionWrite{
		AccountID:  "account-a",
		OwnerID:    first.OwnerID,
		FenceToken: first.FenceToken,
		RequestID:  "command-1",
		Kind:       "command.prepared",
		Payload:    []byte(`{"command":"open"}`),
	}
	receipt, err := store.Append(ctx, firstWrite)
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if receipt.ID == 0 || receipt.Duplicate {
		t.Fatalf("first receipt = %#v, want new non-zero write", receipt)
	}

	clock.Set(clock.Now().Add(2 * time.Minute))
	second, err := store.Acquire(ctx, "account-a", "engine-b")
	if err != nil {
		t.Fatalf("takeover Acquire: %v", err)
	}

	staleWrite := firstWrite
	staleWrite.RequestID = "command-2"
	if _, err := store.Append(ctx, staleWrite); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale Append error = %v, want ErrStaleFence", err)
	}

	currentWrite := firstWrite
	currentWrite.OwnerID = second.OwnerID
	currentWrite.FenceToken = second.FenceToken
	currentWrite.RequestID = "command-2"
	currentReceipt, err := store.Append(ctx, currentWrite)
	if err != nil {
		t.Fatalf("current Append: %v", err)
	}
	if currentReceipt.ID == receipt.ID {
		t.Fatalf("new write reused prior row ID %d", receipt.ID)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_writes WHERE account_id = 'account-a'`).Scan(&rows); err != nil {
		t.Fatalf("count execution writes: %v", err)
	}
	if rows != 2 {
		t.Fatalf("execution write count = %d, want 2", rows)
	}
}

func TestFencedExecutionWriteIsIdempotentButConflictsAreRejected(t *testing.T) {
	store, _, _ := newTestStore(t, time.Minute)
	ctx := context.Background()
	lease, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	write := ExecutionWrite{
		AccountID:  "account-a",
		OwnerID:    lease.OwnerID,
		FenceToken: lease.FenceToken,
		RequestID:  "command-1",
		Kind:       "command.submitted",
		Payload:    []byte("payload"),
	}
	first, err := store.Append(ctx, write)
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}
	second, err := store.Append(ctx, write)
	if err != nil {
		t.Fatalf("idempotent Append: %v", err)
	}
	if !second.Duplicate || second.ID != first.ID {
		t.Fatalf("duplicate receipt = %#v, want ID %d and Duplicate=true", second, first.ID)
	}

	conflict := write
	conflict.Payload = []byte("different")
	if _, err := store.Append(ctx, conflict); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("conflicting Append error = %v, want ErrRequestConflict", err)
	}
}

func TestRenewRejectsExpiredAndSupersededFence(t *testing.T) {
	store, clock, _ := newTestStore(t, time.Minute)
	ctx := context.Background()
	first, err := store.Acquire(ctx, "account-a", "engine-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	clock.Set(clock.Now().Add(2 * time.Minute))
	if _, err := store.Renew(ctx, first); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("expired Renew error = %v, want ErrStaleFence", err)
	}
	second, err := store.Acquire(ctx, "account-a", "engine-b")
	if err != nil {
		t.Fatalf("takeover Acquire: %v", err)
	}
	if _, err := store.Renew(ctx, first); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("superseded Renew error = %v, want ErrStaleFence", err)
	}
	if _, err := store.Renew(ctx, second); err != nil {
		t.Fatalf("current Renew: %v", err)
	}
}

func TestConcurrentAcquireAllowsOneLiveOwner(t *testing.T) {
	store, _, _ := newTestStore(t, time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup
	type result struct {
		owner string
		err   error
	}
	results := make(chan result, 2)
	for _, owner := range []string{"engine-a", "engine-b"} {
		owner := owner
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Acquire(ctx, "account-a", owner)
			results <- result{owner: owner, err: err}
		}()
	}
	wg.Wait()
	close(results)

	var acquired, held int
	for result := range results {
		switch {
		case result.err == nil:
			acquired++
		case errors.Is(result.err, ErrLeaseHeld):
			held++
		default:
			t.Fatalf("owner %s returned unexpected error: %v", result.owner, result.err)
		}
	}
	if acquired != 1 || held != 1 {
		t.Fatalf("concurrent acquisition results = acquired %d, held %d; want one each", acquired, held)
	}
}
