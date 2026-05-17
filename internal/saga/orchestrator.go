// Package saga implements the payment saga orchestrator: it owns the saga
// state machine, the transactional outbox, and reply handling.
package saga

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	orm "github.com/kitti12911/lib-orm/v3"

	"github.com/kitti12911/saga-sandbox/internal/database"
	"github.com/kitti12911/saga-sandbox/internal/messaging"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ErrSagaNotFound is returned when a saga id has no instance.
var ErrSagaNotFound = errors.New("saga not found")

const (
	sagaTypePayment = "payment"
	captureStepName = "capture_payment"
	captureStep     = 0
)

// PaymentRequest is the validated input to StartPayment.
type PaymentRequest struct {
	IdempotencyKey string
	AccountID      string
	Amount         int64
	Currency       string
}

// SagaView is the read model returned by StartPayment/GetSaga.
type SagaView struct {
	SagaID      string
	State       string
	CurrentStep int32
	LastError   string
}

// Orchestrator drives payment sagas over a single participant.
type Orchestrator struct {
	db *orm.DB
}

// New returns an Orchestrator bound to the given database.
func New(db *orm.DB) *Orchestrator {
	return &Orchestrator{db: db}
}

// StartPayment starts (or returns the existing) payment saga for the request's
// idempotency key. Safe to retry: the same key always yields the same saga and
// never enqueues a second capture.
func (o *Orchestrator) StartPayment(ctx context.Context, req PaymentRequest) (SagaView, error) {
	var view SagaView

	err := o.db.Transaction(ctx, func(ctx context.Context) error {
		idb := o.db.IDB(ctx)

		sagaID := uuid.NewString()
		payload, err := json.Marshal(messaging.PaymentPayload{
			AccountID: req.AccountID,
			Amount:    req.Amount,
			Currency:  req.Currency,
		})
		if err != nil {
			return fmt.Errorf("marshal saga payload: %w", err)
		}

		res, err := idb.NewInsert().
			Model(&database.SagaInstance{
				ID:             sagaID,
				SagaType:       sagaTypePayment,
				IdempotencyKey: req.IdempotencyKey,
				State:          database.SagaRunning,
				CurrentStep:    captureStep,
				Payload:        payload,
			}).
			On("CONFLICT (saga_type, idempotency_key) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("insert saga instance: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("saga instance rows affected: %w", err)
		}

		if affected == 0 {
			existing, loadErr := loadInstanceByKey(ctx, idb, req.IdempotencyKey)
			if loadErr != nil {
				return loadErr
			}
			view = toView(existing)
			return nil
		}

		if err := enqueueCapture(ctx, idb, sagaID, req); err != nil {
			return err
		}

		view = SagaView{SagaID: sagaID, State: database.SagaRunning, CurrentStep: captureStep}
		return nil
	})
	if err != nil {
		return SagaView{}, fmt.Errorf("start payment: %w", err)
	}
	return view, nil
}

// GetSaga returns the current state of a saga instance.
func (o *Orchestrator) GetSaga(ctx context.Context, sagaID string) (SagaView, error) {
	instance := new(database.SagaInstance)
	err := o.db.IDB(ctx).NewSelect().
		Model(instance).
		Where("id = ?", sagaID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return SagaView{}, ErrSagaNotFound
	}
	if err != nil {
		return SagaView{}, fmt.Errorf("get saga: %w", err)
	}
	return toView(instance), nil
}

// HandleReply advances the saga from a participant reply. It is idempotent:
// duplicate or late replies for a terminal saga are ignored.
func (o *Orchestrator) HandleReply(ctx context.Context, reply messaging.Reply) error {
	err := o.db.Transaction(ctx, func(ctx context.Context) error {
		idb := o.db.IDB(ctx)

		instance := new(database.SagaInstance)
		loadErr := idb.NewSelect().
			Model(instance).
			Where("id = ?", reply.SagaID).
			For("UPDATE").
			Scan(ctx)
		if errors.Is(loadErr, sql.ErrNoRows) {
			slog.WarnContext(ctx, "reply for unknown saga", "saga_id", reply.SagaID)
			return nil
		}
		if loadErr != nil {
			return fmt.Errorf("load saga for reply: %w", loadErr)
		}

		if database.IsTerminal(instance.State) {
			return nil
		}

		responseJSON, err := json.Marshal(reply)
		if err != nil {
			return fmt.Errorf("marshal reply: %w", err)
		}

		stepStatus := database.StepSucceeded
		sagaState := database.SagaCompleted
		lastError := ""
		if reply.Status != messaging.StatusOK {
			stepStatus = database.StepFailed
			// Capture is the pivot; a failed capture means nothing was
			// captured, so the saga aborts cleanly (no compensation needed).
			sagaState = database.SagaCompensated
			lastError = reply.Error
		}

		if _, err := idb.NewUpdate().
			Model((*database.SagaStep)(nil)).
			Set("status = ?", stepStatus).
			Set("response = ?", responseJSON).
			Set("updated_at = now()").
			Where("saga_id = ?", reply.SagaID).
			Where("step_index = ?", captureStep).
			Where("kind = ?", database.KindForward).
			Exec(ctx); err != nil {
			return fmt.Errorf("update saga step: %w", err)
		}

		if _, err := idb.NewUpdate().
			Model((*database.SagaInstance)(nil)).
			Set("state = ?", sagaState).
			Set("last_error = ?", lastError).
			Set("updated_at = now()").
			Where("id = ?", reply.SagaID).
			Exec(ctx); err != nil {
			return fmt.Errorf("update saga instance: %w", err)
		}

		slog.InfoContext(ctx, "saga advanced",
			"saga_id", reply.SagaID, "state", sagaState, "reply_status", reply.Status)
		return nil
	})
	if err != nil {
		return fmt.Errorf("handle reply: %w", err)
	}
	return nil
}

func enqueueCapture(ctx context.Context, idb bun.IDB, sagaID string, req PaymentRequest) error {
	msgID := uuid.NewString()
	cmd := messaging.Command{
		MsgID:          msgID,
		SagaID:         sagaID,
		IdempotencyKey: req.IdempotencyKey,
		Type:           messaging.TypeCapture,
		Payload: messaging.PaymentPayload{
			AccountID: req.AccountID,
			Amount:    req.Amount,
			Currency:  req.Currency,
		},
	}
	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal capture command: %w", err)
	}

	if _, err := idb.NewInsert().
		Model(&database.SagaStep{
			ID:        uuid.NewString(),
			SagaID:    sagaID,
			StepIndex: captureStep,
			StepName:  captureStepName,
			Kind:      database.KindForward,
			Status:    database.StepPending,
			Request:   cmdJSON,
		}).
		Exec(ctx); err != nil {
		return fmt.Errorf("insert saga step: %w", err)
	}

	if _, err := idb.NewInsert().
		Model(&database.SagaOutbox{
			ID:      uuid.NewString(),
			SagaID:  sagaID,
			Topic:   messaging.SubjectCaptureCmd,
			MsgID:   msgID,
			Payload: cmdJSON,
			Status:  database.OutboxPending,
		}).
		Exec(ctx); err != nil {
		return fmt.Errorf("insert saga outbox: %w", err)
	}
	return nil
}

func loadInstanceByKey(ctx context.Context, idb bun.IDB, idempotencyKey string) (*database.SagaInstance, error) {
	instance := new(database.SagaInstance)
	if err := idb.NewSelect().
		Model(instance).
		Where("saga_type = ?", sagaTypePayment).
		Where("idempotency_key = ?", idempotencyKey).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("load existing saga: %w", err)
	}
	return instance, nil
}

func toView(instance *database.SagaInstance) SagaView {
	return SagaView{
		SagaID:      instance.ID,
		State:       instance.State,
		CurrentStep: instance.CurrentStep,
		LastError:   instance.LastError,
	}
}
