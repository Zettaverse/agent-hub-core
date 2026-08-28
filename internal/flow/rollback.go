package flow

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Effect is a single side effect to be applied (and potentially rolled back).
type Effect struct {
	Kind   string         `json:"kind"` // database, modbus, file, mcp_tool
	Target string         `json:"target"`
	Value  any            `json:"value,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

// EffectRecord is the persisted record of an applied side effect, sufficient
// to undo it.
type EffectRecord struct {
	ID         string
	Effect     Effect
	PriorValue any
}

// SideEffectStore applies and undoes side effects. The in-memory
// implementation (MemorySideEffectStore) is used by tests; a production
// implementation can back database/modbus/file effects with real systems.
type SideEffectStore interface {
	Apply(ctx context.Context, e Effect) (EffectRecord, error)
	Undo(ctx context.Context, rec EffectRecord) error
}

// MemorySideEffectStore is a thread-safe in-memory SideEffectStore. Values are
// keyed by kind+target; Undo restores the prior value.
type MemorySideEffectStore struct {
	mu      sync.Mutex
	data    map[string]any
	undoLog []EffectRecord
}

// NewMemorySideEffectStore returns an empty store.
func NewMemorySideEffectStore() *MemorySideEffectStore {
	return &MemorySideEffectStore{data: make(map[string]any)}
}

func key(kind, target string) string {
	return kind + ":" + target
}

// Apply records the prior value (if any) and writes the new value.
func (m *MemorySideEffectStore) Apply(ctx context.Context, e Effect) (EffectRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(e.Kind, e.Target)
	rec := EffectRecord{
		ID:         uuid.NewString(),
		Effect:     e,
		PriorValue: m.data[k],
	}
	m.data[k] = e.Value
	return rec, nil
}

// Undo restores the prior value captured at Apply time.
func (m *MemorySideEffectStore) Undo(ctx context.Context, rec EffectRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(rec.Effect.Kind, rec.Effect.Target)
	if rec.PriorValue == nil {
		delete(m.data, k)
	} else {
		m.data[k] = rec.PriorValue
	}
	m.undoLog = append(m.undoLog, rec)
	return nil
}

// Value returns the current value for a kind+target pair.
func (m *MemorySideEffectStore) Value(kind, target string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key(kind, target)]
	return v, ok
}

// UndoLog returns the sequence of undone records (in undo order).
func (m *MemorySideEffectStore) UndoLog() []EffectRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]EffectRecord, len(m.undoLog))
	copy(out, m.undoLog)
	return out
}

// rollbackLog accumulates applied side effects in order during a single
// execution so they can be reversed on failure.
type rollbackLog struct {
	store   SideEffectStore
	records []EffectRecord
}

func newRollbackLog(store SideEffectStore) *rollbackLog {
	return &rollbackLog{store: store}
}

// apply applies an effect and records it for later rollback.
func (r *rollbackLog) apply(ctx context.Context, e Effect) error {
	rec, err := r.store.Apply(ctx, e)
	if err != nil {
		return err
	}
	r.records = append(r.records, rec)
	return nil
}

// rollback undoes all applied effects in reverse order.
func (r *rollbackLog) rollback(ctx context.Context) []error {
	var errs []error
	for i := len(r.records) - 1; i >= 0; i-- {
		if err := r.store.Undo(ctx, r.records[i]); err != nil {
			errs = append(errs, err)
		}
	}
	r.records = nil
	return errs
}
