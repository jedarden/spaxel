package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BreakerState is the account-local execution safety state. The values are
// persisted in SQLite, so changing them is a schema/API change.
type BreakerState string

const (
	StateActive     BreakerState = "active"
	StateReduceOnly BreakerState = "reduce_only"
	StateHalted     BreakerState = "halted"

	// RearmAcknowledgement is the typed acknowledgement used by callers that
	// expose an operator rearm form. The Store also enforces operator authority;
	// a phrase alone can never rearm an account.
	RearmAcknowledgement = "REARM"
)

// Aliases make the safety states readable at call sites without introducing
// a second set of persisted values.
const (
	BreakerActive     = StateActive
	BreakerReduceOnly = StateReduceOnly
	BreakerHalted     = StateHalted
)

var (
	ErrBreakerInvalidRequest = errors.New("invalid execution breaker request")
	ErrBreakerHalted         = errors.New("execution is halted")
	ErrBreakerReduceOnly     = errors.New("execution is reduce-only")
	ErrRearmUnauthorized     = errors.New("only an operator may rearm execution")
	ErrRearmAcknowledgement  = errors.New("invalid execution rearm acknowledgement")
	ErrBreakerNotTripped     = errors.New("execution breaker is not tripped")
	ErrBreakerConflict       = errors.New("execution breaker state changed concurrently")
)

// IdentityClass identifies the authenticated caller at the execution
// boundary. An MCP identity may trip a breaker, but it is never an operator.
type IdentityClass string

const (
	IdentityMCP      IdentityClass = "mcp"
	IdentitySystem   IdentityClass = "system"
	IdentityOperator IdentityClass = "operator"
	IdentityOther    IdentityClass = "other"
)

// Identity is the already-authenticated caller supplied by the API layer.
// The breaker does not accept a boolean such as IsOperator: the authority
// class is explicit and is retained in the transition history.
type Identity struct {
	ID    string        `json:"id"`
	Class IdentityClass `json:"class"`
}

// Actor is a descriptive alias for callers that use actor terminology.
type Actor = Identity

// BreakerSnapshot is the durable state returned to execution callers. The
// last trip remains visible after rearm so operators can explain why the
// account stopped.
type BreakerSnapshot struct {
	AccountID      string        `json:"account_id"`
	State          BreakerState  `json:"state"`
	Reason         string        `json:"reason"`
	TriggeredBy    string        `json:"triggered_by"`
	TriggeredClass IdentityClass `json:"triggered_class"`
	TriggeredAt    time.Time     `json:"triggered_at"`
	RearmedBy      string        `json:"rearmed_by"`
	RearmedClass   IdentityClass `json:"rearmed_class"`
	RearmedAt      time.Time     `json:"rearmed_at"`
	Revision       int64         `json:"revision"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type BreakerEvent struct {
	ID            int64         `json:"id"`
	AccountID     string        `json:"account_id"`
	PreviousState BreakerState  `json:"previous_state"`
	State         BreakerState  `json:"state"`
	Kind          string        `json:"kind"`
	ActorID       string        `json:"actor_id"`
	ActorClass    IdentityClass `json:"actor_class"`
	Reason        string        `json:"reason"`
	CreatedAt     time.Time     `json:"created_at"`
}

type HaltRequest struct {
	AccountID string   `json:"account_id"`
	Actor     Identity `json:"actor"`
	Reason    string   `json:"reason"`
}

type ReduceOnlyRequest struct {
	AccountID string   `json:"account_id"`
	Actor     Identity `json:"actor"`
	Reason    string   `json:"reason"`
}

type RearmRequest struct {
	AccountID       string   `json:"account_id"`
	Operator        Identity `json:"operator"`
	Reason          string   `json:"reason"`
	Acknowledgement string   `json:"acknowledgement"`
}

// ExecutionAction describes the safety-relevant effect of a broker command.
// Opening and adding exposure are deliberately separate from management so
// reduce-only cannot be bypassed by a generic command name.
type ExecutionAction string

const (
	ActionOpen        ExecutionAction = "open"
	ActionAddExposure ExecutionAction = "add_exposure"
	ActionReduce      ExecutionAction = "reduce"
	ActionClose       ExecutionAction = "close"
	ActionCancel      ExecutionAction = "cancel"
	ActionModify      ExecutionAction = "modify"
	ActionReconcile   ExecutionAction = "reconcile"
	ActionStatus      ExecutionAction = "status"
)

// Aliases match the language used by execution adapters while keeping one
// action vocabulary for authorization.
const (
	ActionEnter = ActionOpen
	ActionAdd   = ActionAddExposure
	ActionExit  = ActionClose
)

// Allows reports whether an action is allowed under this breaker state. Halt
// and reduce-only are exposure breakers, not a way to hide existing risk:
// closing, reducing, cancelling, modifying an existing order, reconciling,
// and reading status remain available for management.
func (state BreakerState) Allows(action ExecutionAction) bool {
	if !validExecutionAction(action) {
		return false
	}
	switch state {
	case StateActive:
		return true
	case StateReduceOnly, StateHalted:
		return isManagementAction(action)
	default:
		return false
	}
}

// CanManage is the explicit management half of the state matrix.
func (state BreakerState) CanManage(action ExecutionAction) bool {
	if state != StateActive && state != StateReduceOnly && state != StateHalted {
		return false
	}
	return isManagementAction(action)
}

// IsReduceOnly reports whether the state forbids new exposure.
func (state BreakerState) IsReduceOnly() bool {
	return state == StateReduceOnly || state == StateHalted
}

// GetBreakerState returns the durable state. An account with no row is
// implicitly active; the read does not create durable state.
func (s *Store) GetBreakerState(ctx context.Context, accountID string) (BreakerSnapshot, error) {
	if s == nil || s.db == nil {
		return BreakerSnapshot{}, fmt.Errorf("%w: nil store", ErrBreakerInvalidRequest)
	}
	if accountID == "" {
		return BreakerSnapshot{}, fmt.Errorf("%w: account ID is required", ErrBreakerInvalidRequest)
	}
	snapshot, found, err := s.loadBreakerState(ctx, s.db, accountID)
	if err != nil {
		return BreakerSnapshot{}, err
	}
	if !found {
		return BreakerSnapshot{AccountID: accountID, State: StateActive}, nil
	}
	return snapshot, nil
}

// State is a concise alias for GetBreakerState.
func (s *Store) State(ctx context.Context, accountID string) (BreakerSnapshot, error) {
	return s.GetBreakerState(ctx, accountID)
}

// Halt trips the strongest breaker. Any named authenticated identity can
// halt, including an MCP identity. A halt never requires operator authority.
func (s *Store) Halt(ctx context.Context, request HaltRequest) (BreakerSnapshot, error) {
	if err := validateTripRequest(request.AccountID, request.Actor); err != nil {
		return BreakerSnapshot{}, err
	}
	return s.transitionBreaker(ctx, request.AccountID, request.Actor, request.Reason,
		StateHalted, "halt")
}

// ReduceOnly trips the exposure breaker. It cannot weaken an already halted
// account; only an operator rearm can return either state to active.
func (s *Store) ReduceOnly(ctx context.Context, request ReduceOnlyRequest) (BreakerSnapshot, error) {
	if err := validateTripRequest(request.AccountID, request.Actor); err != nil {
		return BreakerSnapshot{}, err
	}
	return s.transitionBreaker(ctx, request.AccountID, request.Actor, request.Reason,
		StateReduceOnly, "reduce_only")
}

// Trip is a convenience entry point for systems that choose the breaker
// level dynamically. The requested state may only become more restrictive.
func (s *Store) Trip(ctx context.Context, accountID string, actor Identity, state BreakerState, reason string) (BreakerSnapshot, error) {
	switch state {
	case StateHalted:
		return s.Halt(ctx, HaltRequest{AccountID: accountID, Actor: actor, Reason: reason})
	case StateReduceOnly:
		return s.ReduceOnly(ctx, ReduceOnlyRequest{AccountID: accountID, Actor: actor, Reason: reason})
	default:
		return BreakerSnapshot{}, fmt.Errorf("%w: trip state must be reduce_only or halted", ErrBreakerInvalidRequest)
	}
}

// Rearm clears either breaker only for an operator-class identity. This is
// the service-side invariant behind the separately authenticated operator API;
// an MCP caller carrying the same request shape receives ErrRearmUnauthorized.
func (s *Store) Rearm(ctx context.Context, request RearmRequest) (BreakerSnapshot, error) {
	if request.AccountID == "" || request.Operator.ID == "" {
		return BreakerSnapshot{}, fmt.Errorf("%w: account ID and operator ID are required", ErrBreakerInvalidRequest)
	}
	if request.Operator.Class != IdentityOperator {
		return BreakerSnapshot{}, ErrRearmUnauthorized
	}
	if request.Acknowledgement != RearmAcknowledgement {
		return BreakerSnapshot{}, ErrRearmAcknowledgement
	}

	if s == nil || s.db == nil {
		return BreakerSnapshot{}, fmt.Errorf("%w: nil store", ErrBreakerInvalidRequest)
	}
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BreakerSnapshot{}, fmt.Errorf("begin breaker rearm: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, found, err := s.loadBreakerState(ctx, tx, request.AccountID)
	if err != nil {
		return BreakerSnapshot{}, err
	}
	if !found || current.State == StateActive {
		return BreakerSnapshot{}, ErrBreakerNotTripped
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE execution_breaker_states
		SET state = 'active', rearmed_by = ?, rearmed_kind = ?, rearmed_at = ?,
			revision = revision + 1, updated_at = ?
		WHERE account_id = ? AND revision = ?
	`, request.Operator.ID, string(request.Operator.Class), now.UnixMilli(), now.UnixMilli(),
		request.AccountID, current.Revision)
	if err != nil {
		return BreakerSnapshot{}, fmt.Errorf("rearm execution breaker: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return BreakerSnapshot{}, fmt.Errorf("rearm execution breaker rows affected: %w", err)
	} else if affected != 1 {
		return BreakerSnapshot{}, ErrBreakerConflict
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO execution_breaker_events
			(account_id, previous_state, state, event_kind, actor_id, actor_kind, reason, created_at)
		VALUES (?, ?, 'active', 'rearm', ?, ?, ?, ?)
	`, request.AccountID, string(current.State), request.Operator.ID,
		string(request.Operator.Class), request.Reason, now.UnixMilli()); err != nil {
		return BreakerSnapshot{}, fmt.Errorf("record breaker rearm: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BreakerSnapshot{}, fmt.Errorf("commit breaker rearm: %w", err)
	}

	current.State = StateActive
	current.RearmedBy = request.Operator.ID
	current.RearmedClass = request.Operator.Class
	current.RearmedAt = now
	current.Revision++
	current.UpdatedAt = now
	return current, nil
}

// ClearHalt is intentionally an operator-authority alias. It does not expose
// a second recovery path and therefore cannot be called by an MCP identity.
func (s *Store) ClearHalt(ctx context.Context, request RearmRequest) (BreakerSnapshot, error) {
	return s.Rearm(ctx, request)
}

// OperatorAPI returns the only named recovery facade. The underlying Store
// still checks the identity class, so passing an MCP identity cannot bypass
// the boundary by obtaining this value from a different call site.
func (s *Store) OperatorAPI() *OperatorAPI {
	return &OperatorAPI{store: s}
}

type OperatorAPI struct {
	store *Store
}

func (a *OperatorAPI) Rearm(ctx context.Context, request RearmRequest) (BreakerSnapshot, error) {
	if a == nil || a.store == nil {
		return BreakerSnapshot{}, fmt.Errorf("%w: nil operator API", ErrBreakerInvalidRequest)
	}
	return a.store.Rearm(ctx, request)
}

// Authorize checks the persisted state at the command boundary. Callers must
// invoke it immediately before mutation, alongside their normal fresh-state
// and approval checks.
func (s *Store) Authorize(ctx context.Context, accountID string, action ExecutionAction) error {
	state, err := s.GetBreakerState(ctx, accountID)
	if err != nil {
		return err
	}
	if !validExecutionAction(action) {
		return fmt.Errorf("%w: unsupported action %q", ErrBreakerInvalidRequest, action)
	}
	if state.State == StateHalted && !isManagementAction(action) {
		return ErrBreakerHalted
	}
	if state.State == StateReduceOnly && isExposureIncreasing(action) {
		return ErrBreakerReduceOnly
	}
	if !state.State.Allows(action) {
		return ErrBreakerHalted
	}
	return nil
}

// CanExecute is a non-database convenience for code that already has a
// snapshot. It mirrors Authorize's state matrix without the read.
func (s BreakerSnapshot) CanExecute(action ExecutionAction) bool {
	return s.State.Allows(action)
}

// BreakerEvents returns the append-only transition history in insertion order.
func (s *Store) BreakerEvents(ctx context.Context, accountID string) ([]BreakerEvent, error) {
	if s == nil || s.db == nil || accountID == "" {
		return nil, fmt.Errorf("%w: account ID and store are required", ErrBreakerInvalidRequest)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, previous_state, state, event_kind, actor_id, actor_kind, reason, created_at
		FROM execution_breaker_events WHERE account_id = ? ORDER BY id
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("load breaker events: %w", err)
	}
	defer rows.Close()

	var events []BreakerEvent
	for rows.Next() {
		var (
			event                      BreakerEvent
			previous, state, kind      string
			actorID, actorKind, reason string
			createdAt                  int64
		)
		if err := rows.Scan(&event.ID, &previous, &state, &kind, &actorID, &actorKind, &reason, &createdAt); err != nil {
			return nil, fmt.Errorf("scan breaker event: %w", err)
		}
		event.AccountID = accountID
		event.PreviousState = BreakerState(previous)
		event.State = BreakerState(state)
		event.Kind = kind
		event.ActorID = actorID
		event.ActorClass = IdentityClass(actorKind)
		event.Reason = reason
		event.CreatedAt = time.UnixMilli(createdAt).UTC()
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate breaker events: %w", err)
	}
	return events, nil
}

func (s *Store) transitionBreaker(ctx context.Context, accountID string, actor Identity, reason string, requested BreakerState, kind string) (BreakerSnapshot, error) {
	if s == nil || s.db == nil {
		return BreakerSnapshot{}, fmt.Errorf("%w: nil store", ErrBreakerInvalidRequest)
	}
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BreakerSnapshot{}, fmt.Errorf("begin breaker transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, found, err := s.loadBreakerState(ctx, tx, accountID)
	if err != nil {
		return BreakerSnapshot{}, err
	}
	if !found {
		current = BreakerSnapshot{AccountID: accountID, State: StateActive}
	}
	effective := requested
	if current.State == StateHalted {
		effective = StateHalted
	}

	if !found {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO execution_breaker_states
				(account_id, state, reason, triggered_by, triggered_kind, triggered_at,
				 rearmed_by, rearmed_kind, rearmed_at, revision, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, '', '', 0, 1, ?)
		`, accountID, string(effective), reason, actor.ID, string(actor.Class), now.UnixMilli(), now.UnixMilli()); err != nil {
			return BreakerSnapshot{}, fmt.Errorf("insert execution breaker state: %w", err)
		}
	} else {
		result, err := tx.ExecContext(ctx, `
			UPDATE execution_breaker_states
			SET state = CASE WHEN state = 'halted' THEN 'halted' ELSE ? END,
				reason = CASE WHEN state = 'halted' THEN reason ELSE ? END,
				triggered_by = CASE WHEN state = 'halted' THEN triggered_by ELSE ? END,
				triggered_kind = CASE WHEN state = 'halted' THEN triggered_kind ELSE ? END,
				triggered_at = CASE WHEN state = 'halted' THEN triggered_at ELSE ? END,
				rearmed_by = CASE WHEN state = 'halted' THEN rearmed_by ELSE '' END,
				rearmed_kind = CASE WHEN state = 'halted' THEN rearmed_kind ELSE '' END,
				rearmed_at = CASE WHEN state = 'halted' THEN rearmed_at ELSE 0 END,
				revision = revision + 1, updated_at = ?
			WHERE account_id = ? AND revision = ?
		`, string(effective), reason, actor.ID, string(actor.Class), now.UnixMilli(), now.UnixMilli(), accountID, current.Revision)
		if err != nil {
			return BreakerSnapshot{}, fmt.Errorf("update execution breaker state: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return BreakerSnapshot{}, fmt.Errorf("update execution breaker state rows affected: %w", err)
		} else if affected != 1 {
			return BreakerSnapshot{}, ErrBreakerConflict
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO execution_breaker_events
			(account_id, previous_state, state, event_kind, actor_id, actor_kind, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, accountID, string(current.State), string(effective), kind, actor.ID, string(actor.Class), reason, now.UnixMilli()); err != nil {
		return BreakerSnapshot{}, fmt.Errorf("record breaker transition: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BreakerSnapshot{}, fmt.Errorf("commit breaker transition: %w", err)
	}

	if !found {
		current.Revision = 1
	} else {
		current.Revision++
	}
	if current.State != StateHalted {
		current.State = effective
		current.Reason = reason
		current.TriggeredBy = actor.ID
		current.TriggeredClass = actor.Class
		current.TriggeredAt = now
		current.RearmedBy = ""
		current.RearmedClass = ""
		current.RearmedAt = time.Time{}
	}
	current.UpdatedAt = now
	return current, nil
}

type breakerQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) loadBreakerState(ctx context.Context, q breakerQuerier, accountID string) (BreakerSnapshot, bool, error) {
	var (
		state, reason, triggeredBy, triggeredKind   string
		triggeredAt, rearmedAt, revision, updatedAt int64
		rearmedBy, rearmedKind                      string
	)
	err := q.QueryRowContext(ctx, `
		SELECT state, reason, triggered_by, triggered_kind, triggered_at,
			rearmed_by, rearmed_kind, rearmed_at, revision, updated_at
		FROM execution_breaker_states WHERE account_id = ?
	`, accountID).Scan(&state, &reason, &triggeredBy, &triggeredKind, &triggeredAt,
		&rearmedBy, &rearmedKind, &rearmedAt, &revision, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BreakerSnapshot{}, false, nil
	}
	if err != nil {
		return BreakerSnapshot{}, false, fmt.Errorf("load execution breaker state: %w", err)
	}
	return BreakerSnapshot{
		AccountID:      accountID,
		State:          BreakerState(state),
		Reason:         reason,
		TriggeredBy:    triggeredBy,
		TriggeredClass: IdentityClass(triggeredKind),
		TriggeredAt:    unixMillisOrZero(triggeredAt),
		RearmedBy:      rearmedBy,
		RearmedClass:   IdentityClass(rearmedKind),
		RearmedAt:      unixMillisOrZero(rearmedAt),
		Revision:       revision,
		UpdatedAt:      unixMillisOrZero(updatedAt),
	}, true, nil
}

func validateTripRequest(accountID string, actor Identity) error {
	if accountID == "" || strings.TrimSpace(actor.ID) == "" {
		return fmt.Errorf("%w: account ID and actor ID are required", ErrBreakerInvalidRequest)
	}
	return nil
}

func validExecutionAction(action ExecutionAction) bool {
	switch action {
	case ActionOpen, ActionAddExposure, ActionReduce, ActionClose, ActionCancel,
		ActionModify, ActionReconcile, ActionStatus:
		return true
	default:
		return false
	}
}

func isExposureIncreasing(action ExecutionAction) bool {
	return action == ActionOpen || action == ActionAddExposure
}

func isManagementAction(action ExecutionAction) bool {
	switch action {
	case ActionReduce, ActionClose, ActionCancel, ActionModify, ActionReconcile, ActionStatus:
		return true
	default:
		return false
	}
}

func unixMillisOrZero(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}
