package saga

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	orm "github.com/kitti12911/lib-orm/v3"

	"github.com/kitti12911/saga-sandbox/internal/database"
	"github.com/kitti12911/saga-sandbox/internal/messaging"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	defaultSweepInterval = 15 * time.Second
	defaultStuckAfter    = 60 * time.Second
	defaultMaxAttempts   = 5
	defaultSweepBatch    = 100
)

// sweepAction is the decision taken for a stuck saga.
type sweepAction int

const (
	actionRedrive sweepAction = iota
	actionEscalate
)

// decideSweepAction picks redrive vs escalate from the step attempt count.
// Pure and unit-tested; the DB-touching logic is kept separate.
func decideSweepAction(attempts, maxAttempts int) sweepAction {
	if attempts >= maxAttempts {
		return actionEscalate
	}
	return actionRedrive
}

// Sweeper is the always-on recovery backstop: it finds sagas that have been
// RUNNING too long and either re-drives the pending step or escalates them to
// NEEDS_INTERVENTION so nothing fails silently.
type Sweeper struct {
	db          *orm.DB
	interval    time.Duration
	stuckAfter  time.Duration
	maxAttempts int
	batchSize   int
}

// NewSweeper builds a Sweeper, applying defaults to zero-valued options.
func NewSweeper(db *orm.DB, interval, stuckAfter time.Duration, maxAttempts, batchSize int) *Sweeper {
	if interval <= 0 {
		interval = defaultSweepInterval
	}
	if stuckAfter <= 0 {
		stuckAfter = defaultStuckAfter
	}
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	if batchSize <= 0 {
		batchSize = defaultSweepBatch
	}
	return &Sweeper{
		db:          db,
		interval:    interval,
		stuckAfter:  stuckAfter,
		maxAttempts: maxAttempts,
		batchSize:   batchSize,
	}
}

// Run sweeps on a fixed interval until the context is canceled.
func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.sweepOnce(ctx); err != nil {
				slog.ErrorContext(ctx, "saga sweep failed", "error", err)
			}
		}
	}
}

func (s *Sweeper) sweepOnce(ctx context.Context) error {
	cutoff := time.Now().Add(-s.stuckAfter)

	var stuck []database.SagaInstance
	if err := s.db.IDB(ctx).NewSelect().
		Model(&stuck).
		Where("state = ?", database.SagaRunning).
		Where("updated_at < ?", cutoff).
		Order("updated_at ASC").
		Limit(s.batchSize).
		Scan(ctx); err != nil {
		return fmt.Errorf("select stuck sagas: %w", err)
	}

	for i := range stuck {
		if err := s.recover(ctx, stuck[i].ID); err != nil {
			slog.ErrorContext(ctx, "saga recovery failed",
				"saga_id", stuck[i].ID, "error", err)
		}
	}
	return nil
}

// recover loads one stuck saga under FOR UPDATE and either re-drives its
// pending forward step or escalates the saga to NEEDS_INTERVENTION.
func (s *Sweeper) recover(ctx context.Context, sagaID string) error {
	err := s.db.Transaction(ctx, func(ctx context.Context) error {
		idb := s.db.IDB(ctx)

		instance := new(database.SagaInstance)
		loadErr := idb.NewSelect().
			Model(instance).
			Where("id = ?", sagaID).
			For("UPDATE").
			Scan(ctx)
		if errors.Is(loadErr, sql.ErrNoRows) {
			return nil
		}
		if loadErr != nil {
			return fmt.Errorf("load saga: %w", loadErr)
		}

		// A reply may have advanced the saga between select and lock.
		if instance.State != database.SagaRunning {
			return nil
		}

		step := new(database.SagaStep)
		if err := idb.NewSelect().
			Model(step).
			Where("saga_id = ?", sagaID).
			Where("step_index = ?", captureStep).
			Where("kind = ?", database.KindForward).
			Scan(ctx); err != nil {
			return fmt.Errorf("load forward step: %w", err)
		}

		switch decideSweepAction(step.Attempts, s.maxAttempts) {
		case actionEscalate:
			return escalate(ctx, idb, sagaID, step.Attempts)
		default:
			return redrive(ctx, idb, sagaID, step)
		}
	})
	if err != nil {
		return fmt.Errorf("recover saga %s: %w", sagaID, err)
	}
	return nil
}

// escalate marks the saga NEEDS_INTERVENTION; this is the surfaced backstop
// for ops/alerting (slog ERROR with the saga id + audit trail in the row).
func escalate(ctx context.Context, idb bun.IDB, sagaID string, attempts int) error {
	msg := fmt.Sprintf("max re-drive attempts (%d) exhausted", attempts)
	if _, err := idb.NewUpdate().
		Model((*database.SagaInstance)(nil)).
		Set("state = ?", database.SagaNeedsIntervention).
		Set("last_error = ?", msg).
		Set("updated_at = now()").
		Where("id = ?", sagaID).
		Exec(ctx); err != nil {
		return fmt.Errorf("escalate saga: %w", err)
	}
	slog.ErrorContext(ctx, "saga needs intervention",
		"saga_id", sagaID, "attempts", attempts)
	return nil
}

// redrive enqueues a fresh outbox command (new msg_id) for the stuck step.
// The participant's inbox dedups its own msg_ids; the business effect is
// idempotent on idempotency_key, so re-driving is safe.
func redrive(ctx context.Context, idb bun.IDB, sagaID string, step *database.SagaStep) error {
	if len(step.Request) == 0 {
		return fmt.Errorf("step %s has no request payload", step.ID)
	}

	var cmd messaging.Command
	if err := json.Unmarshal(step.Request, &cmd); err != nil {
		return fmt.Errorf("unmarshal step command: %w", err)
	}

	cmd.MsgID = uuid.NewString()
	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal redrive command: %w", err)
	}

	if _, err := idb.NewInsert().
		Model(&database.SagaOutbox{
			ID:      uuid.NewString(),
			SagaID:  sagaID,
			Topic:   messaging.SubjectCaptureCmd,
			MsgID:   cmd.MsgID,
			Payload: cmdJSON,
			Status:  database.OutboxPending,
		}).
		Exec(ctx); err != nil {
		return fmt.Errorf("insert redrive outbox: %w", err)
	}

	if _, err := idb.NewUpdate().
		Model((*database.SagaStep)(nil)).
		Set("attempts = attempts + 1").
		Set("updated_at = now()").
		Where("id = ?", step.ID).
		Exec(ctx); err != nil {
		return fmt.Errorf("bump step attempts: %w", err)
	}

	// Touch saga.updated_at so the sweeper doesn't immediately re-sweep it.
	if _, err := idb.NewUpdate().
		Model((*database.SagaInstance)(nil)).
		Set("updated_at = now()").
		Where("id = ?", sagaID).
		Exec(ctx); err != nil {
		return fmt.Errorf("touch saga: %w", err)
	}

	slog.WarnContext(ctx, "re-driving stuck saga",
		"saga_id", sagaID, "attempts", step.Attempts+1, "msg_id", cmd.MsgID)
	return nil
}
