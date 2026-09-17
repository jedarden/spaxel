package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBreakerStateMatrix(t *testing.T) {
	tests := []struct {
		name   string
		state  BreakerState
		action ExecutionAction
		allow  bool
	}{
		{name: "active opens", state: StateActive, action: ActionOpen, allow: true},
		{name: "active manages", state: StateActive, action: ActionClose, allow: true},
		{name: "reduce only rejects opens", state: StateReduceOnly, action: ActionOpen, allow: false},
		{name: "reduce only rejects adds", state: StateReduceOnly, action: ActionAddExposure, allow: false},
		{name: "reduce only permits closes", state: StateReduceOnly, action: ActionClose, allow: true},
		{name: "reduce only permits cancels", state: StateReduceOnly, action: ActionCancel, allow: true},
		{name: "halt rejects opens", state: StateHalted, action: ActionOpen, allow: false},
		{name: "halt permits close management", state: StateHalted, action: ActionClose, allow: true},
		{name: "halt permits cancel management", state: StateHalted, action: ActionCancel, allow: true},
		{name: "unknown action denied", state: StateActive, action: ExecutionAction("exercise"), allow: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Allows(tt.action); got != tt.allow {
				t.Fatalf("Allows(%q, %q) = %v, want %v", tt.state, tt.action, got, tt.allow)
			}
		})
	}
}

func TestBreakerTransitionsAreDurableAndAsymmetric(t *testing.T) {
	store, clock, _ := newTestStore(t, time.Minute)
	ctx := context.Background()

	initial, err := store.GetBreakerState(ctx, "account-a")
	if err != nil {
		t.Fatalf("initial state: %v", err)
	}
	if initial.State != StateActive || initial.Revision != 0 {
		t.Fatalf("initial state = %#v, want implicit active revision 0", initial)
	}

	clock.Set(clock.Now().Add(time.Second))
	reduce, err := store.ReduceOnly(ctx, ReduceOnlyRequest{
		AccountID: "account-a",
		Actor:     Identity{ID: "risk-monitor", Class: IdentitySystem},
		Reason:    "daily loss threshold",
	})
	if err != nil {
		t.Fatalf("reduce-only trip: %v", err)
	}
	if reduce.State != StateReduceOnly || reduce.Revision != 1 {
		t.Fatalf("reduce-only state = %#v", reduce)
	}
	if err := store.Authorize(ctx, "account-a", ActionOpen); !errors.Is(err, ErrBreakerReduceOnly) {
		t.Fatalf("open in reduce-only error = %v, want ErrBreakerReduceOnly", err)
	}
	for _, action := range []ExecutionAction{ActionReduce, ActionClose, ActionCancel, ActionModify, ActionReconcile} {
		if err := store.Authorize(ctx, "account-a", action); err != nil {
			t.Errorf("management action %q in reduce-only: %v", action, err)
		}
	}

	clock.Set(clock.Now().Add(time.Second))
	halted, err := store.Halt(ctx, HaltRequest{
		AccountID: "account-a",
		Actor:     Identity{ID: "mcp-risk-watcher", Class: IdentityMCP},
		Reason:    "market data became stale",
	})
	if err != nil {
		t.Fatalf("halt: %v", err)
	}
	if halted.State != StateHalted || halted.Revision != 2 {
		t.Fatalf("halted state = %#v", halted)
	}
	if err := store.Authorize(ctx, "account-a", ActionOpen); !errors.Is(err, ErrBreakerHalted) {
		t.Fatalf("open while halted error = %v, want ErrBreakerHalted", err)
	}
	if err := store.Authorize(ctx, "account-a", ActionClose); err != nil {
		t.Fatalf("close while halted: %v", err)
	}

	_, err = store.Rearm(ctx, RearmRequest{
		AccountID: "account-a",
		Operator:  Identity{ID: "trading-mcp", Class: IdentityMCP},
		Reason:    "mcp must not clear safety state",
	})
	if !errors.Is(err, ErrRearmUnauthorized) {
		t.Fatalf("MCP rearm error = %v, want ErrRearmUnauthorized", err)
	}
	stillHalted, err := store.GetBreakerState(ctx, "account-a")
	if err != nil {
		t.Fatalf("state after denied rearm: %v", err)
	}
	if stillHalted.State != StateHalted || stillHalted.Revision != 2 {
		t.Fatalf("denied rearm changed state = %#v", stillHalted)
	}

	clock.Set(clock.Now().Add(time.Second))
	rearmed, err := store.OperatorAPI().Rearm(ctx, RearmRequest{
		AccountID:       "account-a",
		Operator:        Identity{ID: "human-operator", Class: IdentityOperator},
		Reason:          "session and order book rechecked",
		Acknowledgement: RearmAcknowledgement,
	})
	if err != nil {
		t.Fatalf("operator rearm: %v", err)
	}
	if rearmed.State != StateActive || rearmed.Revision != 3 {
		t.Fatalf("rearmed state = %#v", rearmed)
	}
	if rearmed.Reason != "market data became stale" || rearmed.RearmedBy != "human-operator" {
		t.Fatalf("rearm audit fields = %#v", rearmed)
	}
	if err := store.Authorize(ctx, "account-a", ActionOpen); err != nil {
		t.Fatalf("open after operator rearm: %v", err)
	}

	events, err := store.BreakerEvents(ctx, "account-a")
	if err != nil {
		t.Fatalf("breaker events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	for i, want := range []struct {
		previous BreakerState
		state    BreakerState
		kind     string
		actor    string
	}{
		{StateActive, StateReduceOnly, "reduce_only", "risk-monitor"},
		{StateReduceOnly, StateHalted, "halt", "mcp-risk-watcher"},
		{StateHalted, StateActive, "rearm", "human-operator"},
	} {
		if events[i].PreviousState != want.previous || events[i].State != want.state ||
			events[i].Kind != want.kind || events[i].ActorID != want.actor {
			t.Errorf("event %d = %#v, want previous=%q state=%q kind=%q actor=%q", i, events[i], want.previous, want.state, want.kind, want.actor)
		}
	}
}

func TestHaltCannotWeakenAndRearmRequiresOperator(t *testing.T) {
	store, _, _ := newTestStore(t, time.Minute)
	ctx := context.Background()

	if _, err := store.Halt(ctx, HaltRequest{AccountID: "account-a", Actor: Identity{ID: "watcher", Class: IdentityOther}}); err != nil {
		t.Fatalf("initial halt: %v", err)
	}
	state, err := store.ReduceOnly(ctx, ReduceOnlyRequest{
		AccountID: "account-a",
		Actor:     Identity{ID: "mcp", Class: IdentityMCP},
		Reason:    "second trigger must not weaken halt",
	})
	if err != nil {
		t.Fatalf("reduce-only while halted: %v", err)
	}
	if state.State != StateHalted {
		t.Fatalf("reduce-only weakened halted state: %#v", state)
	}

	for _, class := range []IdentityClass{IdentityMCP, IdentitySystem, IdentityOther} {
		_, err := store.Rearm(ctx, RearmRequest{
			AccountID: "account-a",
			Operator:  Identity{ID: "not-operator", Class: class},
		})
		if !errors.Is(err, ErrRearmUnauthorized) {
			t.Errorf("rearm class %q error = %v, want ErrRearmUnauthorized", class, err)
		}
	}

	_, err = store.Rearm(ctx, RearmRequest{
		AccountID: "account-a",
		Operator:  Identity{ID: "human-operator", Class: IdentityOperator},
	})
	if !errors.Is(err, ErrRearmAcknowledgement) {
		t.Fatalf("missing rearm acknowledgement error = %v, want ErrRearmAcknowledgement", err)
	}
	state, err = store.GetBreakerState(ctx, "account-a")
	if err != nil {
		t.Fatalf("state after missing acknowledgement: %v", err)
	}
	if state.State != StateHalted {
		t.Fatalf("missing acknowledgement changed state: %#v", state)
	}
}

func TestBreakerValidation(t *testing.T) {
	store, _, _ := newTestStore(t, time.Minute)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "halt missing account", call: func() error {
			_, err := store.Halt(ctx, HaltRequest{Actor: Identity{ID: "watcher"}})
			return err
		}},
		{name: "halt missing actor", call: func() error {
			_, err := store.Halt(ctx, HaltRequest{AccountID: "account-a"})
			return err
		}},
		{name: "trip cannot select active", call: func() error {
			_, err := store.Trip(ctx, "account-a", Identity{ID: "watcher"}, StateActive, "")
			return err
		}},
		{name: "unknown action", call: func() error {
			return store.Authorize(ctx, "account-a", ExecutionAction("exercise"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrBreakerInvalidRequest) {
				t.Fatalf("error = %v, want ErrBreakerInvalidRequest", err)
			}
		})
	}
}
