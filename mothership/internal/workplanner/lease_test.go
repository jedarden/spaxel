package workplanner

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

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newTestStore(t *testing.T, cfg Config) (*Store, *testClock, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "planner.db"))+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &testClock{now: time.UnixMilli(2_000_000).UTC()}
	cfg.Clock = clock.Now
	store, err := NewStoreWithConfig(db, cfg)
	if err != nil {
		t.Fatalf("new planner store: %v", err)
	}
	return store, clock, db
}

func TestAcquireLeaseTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		wantErr error
	}{
		{name: "missing owner", wantErr: ErrInvalidRequest},
		{name: "valid", owner: "planner-a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _, _ := newTestStore(t, Config{LeaseTTL: time.Minute})
			lease, err := store.Acquire(context.Background(), tt.owner)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Acquire error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			if lease.FenceToken != 1 || lease.OwnerID != tt.owner {
				t.Fatalf("lease = %#v, want first fence and owner", lease)
			}
		})
	}
}

func TestLeaseTakeoverFencesDeadPlanner(t *testing.T) {
	store, clock, _ := newTestStore(t, Config{LeaseTTL: time.Minute})
	ctx := context.Background()
	first, err := store.Acquire(ctx, "planner-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if _, err := store.Acquire(ctx, "planner-b"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("live second Acquire error = %v, want ErrLeaseHeld", err)
	}

	clock.Advance(2 * time.Minute)
	second, err := store.Acquire(ctx, "planner-b")
	if err != nil {
		t.Fatalf("takeover Acquire: %v", err)
	}
	if second.FenceToken != first.FenceToken+1 {
		t.Fatalf("takeover fence = %d, want %d", second.FenceToken, first.FenceToken+1)
	}
	if _, err := store.Renew(ctx, first); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale Renew error = %v, want ErrStaleFence", err)
	}
	item := WorkItem{ID: "item-1", DedupeKey: "request-1", Kind: "collect", Payload: []byte("payload")}
	if _, err := store.Enqueue(ctx, first, item); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale Enqueue error = %v, want ErrStaleFence", err)
	}
	if _, err := store.Enqueue(ctx, second, item); err != nil {
		t.Fatalf("replacement Enqueue: %v", err)
	}
}

func TestSameOwnerHeartbeatKeepsFenceAndReleaseAdvancesIt(t *testing.T) {
	store, clock, _ := newTestStore(t, Config{LeaseTTL: time.Minute})
	ctx := context.Background()
	first, err := store.Acquire(ctx, "planner-a")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	clock.Advance(10 * time.Second)
	renewed, err := store.Acquire(ctx, "planner-a")
	if err != nil {
		t.Fatalf("same-owner Acquire: %v", err)
	}
	if renewed.FenceToken != first.FenceToken || !renewed.ExpiresAt.After(first.ExpiresAt) {
		t.Fatalf("renewed lease = %#v, want same fence and later expiry", renewed)
	}
	if err := store.Release(ctx, renewed); err != nil {
		t.Fatalf("Release: %v", err)
	}
	next, err := store.Acquire(ctx, "planner-b")
	if err != nil {
		t.Fatalf("post-release Acquire: %v", err)
	}
	if next.FenceToken != first.FenceToken+1 {
		t.Fatalf("post-release fence = %d, want %d", next.FenceToken, first.FenceToken+1)
	}
}

func TestEnqueueIsIdempotentAcrossPlannerHandoff(t *testing.T) {
	store, clock, _ := newTestStore(t, Config{LeaseTTL: time.Minute})
	ctx := context.Background()
	first, err := store.Acquire(ctx, "planner-a")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	item := WorkItem{ID: "item-1", DedupeKey: "request-1", Kind: "collect", Payload: []byte("payload")}
	original, err := store.Enqueue(ctx, first, item)
	if err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	clock.Advance(2 * time.Minute)
	second, err := store.Acquire(ctx, "planner-b")
	if err != nil {
		t.Fatalf("takeover Acquire: %v", err)
	}
	duplicate, err := store.Enqueue(ctx, second, item)
	if err != nil {
		t.Fatalf("handoff Enqueue: %v", err)
	}
	if !duplicate.Duplicate || duplicate.Item.ID != original.Item.ID {
		t.Fatalf("handoff receipt = %#v, want duplicate of %q", duplicate, original.Item.ID)
	}
	conflict := item
	conflict.Payload = []byte("changed")
	if _, err := store.Enqueue(ctx, second, conflict); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("conflicting Enqueue error = %v, want ErrRequestConflict", err)
	}
	differentID := item
	differentID.ID = "item-2"
	if _, err := store.Enqueue(ctx, second, differentID); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("duplicate dedupe Enqueue error = %v, want ErrRequestConflict", err)
	}
}

func TestClaimHandoffRejectsStaleWorkerAndResumesItem(t *testing.T) {
	store, clock, _ := newTestStore(t, Config{LeaseTTL: time.Minute, ClaimTTL: time.Minute})
	ctx := context.Background()
	planner, err := store.Acquire(ctx, "planner-a")
	if err != nil {
		t.Fatalf("planner Acquire: %v", err)
	}
	if _, err := store.Enqueue(ctx, planner, WorkItem{ID: "item-1", DedupeKey: "request-1", Kind: "collect"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	first, _, err := store.ClaimNext(ctx, "worker-a")
	if err != nil {
		t.Fatalf("first ClaimNext: %v", err)
	}
	clock.Advance(2 * time.Minute)
	second, record, err := store.ClaimNext(ctx, "worker-b")
	if err != nil {
		t.Fatalf("takeover ClaimNext: %v", err)
	}
	if second.ClaimToken <= first.ClaimToken || record.State != StateLeased {
		t.Fatalf("replacement claim = %#v, record = %#v", second, record)
	}
	if err := store.Complete(ctx, first); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale Complete error = %v, want ErrStaleClaim", err)
	}
	if err := store.Complete(ctx, second); err != nil {
		t.Fatalf("replacement Complete: %v", err)
	}
	final, found, err := store.Get(ctx, "item-1")
	if err != nil || !found {
		t.Fatalf("Get completed item: found=%v err=%v", found, err)
	}
	if final.State != StateCompleted || final.AttemptCount != 2 {
		t.Fatalf("final item = %#v, want completed after two attempts", final)
	}
}

func TestPlannerStateSurvivesStoreReplacement(t *testing.T) {
	store, _, db := newTestStore(t, Config{LeaseTTL: time.Minute})
	ctx := context.Background()
	lease, err := store.Acquire(ctx, "planner-a")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := store.Enqueue(ctx, lease, WorkItem{ID: "item-1", DedupeKey: "request-1", Kind: "persisted", Payload: []byte("durable")}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	replacement, err := NewStoreWithConfig(db, Config{LeaseTTL: time.Minute, Clock: store.clock})
	if err != nil {
		t.Fatalf("replacement store: %v", err)
	}
	record, found, err := replacement.Get(ctx, "item-1")
	if err != nil || !found {
		t.Fatalf("replacement Get: found=%v err=%v", found, err)
	}
	if string(record.Payload) != "durable" || record.State != StateQueued {
		t.Fatalf("replacement record = %#v, want durable queued item", record)
	}
}

func TestConcurrentPlannerAcquireAllowsOneOwner(t *testing.T) {
	store, _, _ := newTestStore(t, Config{LeaseTTL: time.Minute})
	ctx := context.Background()
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, owner := range []string{"planner-a", "planner-b"} {
		owner := owner
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Acquire(ctx, owner)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var acquired, held int
	for err := range results {
		switch {
		case err == nil:
			acquired++
		case errors.Is(err, ErrLeaseHeld):
			held++
		default:
			t.Fatalf("unexpected concurrent Acquire error: %v", err)
		}
	}
	if acquired != 1 || held != 1 {
		t.Fatalf("concurrent acquisition = acquired %d held %d, want one each", acquired, held)
	}
}
