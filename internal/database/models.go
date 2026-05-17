package database

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// SagaInstance is one saga execution, unique on (saga_type, idempotency_key).
type SagaInstance struct {
	bun.BaseModel `bun:"table:saga_instances"`

	ID             string          `bun:"id,pk"`
	SagaType       string          `bun:"saga_type,notnull"`
	IdempotencyKey string          `bun:"idempotency_key,notnull"`
	State          string          `bun:"state,notnull"`
	CurrentStep    int32           `bun:"current_step,notnull"`
	Payload        json.RawMessage `bun:"payload,type:jsonb,notnull"`
	LastError      string          `bun:"last_error"`
	CreatedAt      time.Time       `bun:"created_at,nullzero,notnull,default:now()"`
	UpdatedAt      time.Time       `bun:"updated_at,nullzero,notnull,default:now()"`
}

// SagaStep is a per-step record for audit and recovery.
type SagaStep struct {
	bun.BaseModel `bun:"table:saga_steps"`

	ID        string          `bun:"id,pk"`
	SagaID    string          `bun:"saga_id,notnull"`
	StepIndex int             `bun:"step_index,notnull"`
	StepName  string          `bun:"step_name,notnull"`
	Kind      string          `bun:"kind,notnull"`
	Status    string          `bun:"status,notnull"`
	Attempts  int             `bun:"attempts,notnull"`
	Request   json.RawMessage `bun:"request,type:jsonb"`
	Response  json.RawMessage `bun:"response,type:jsonb"`
	CreatedAt time.Time       `bun:"created_at,nullzero,notnull,default:now()"`
	UpdatedAt time.Time       `bun:"updated_at,nullzero,notnull,default:now()"`
}

// SagaOutbox holds commands to be published by the relay.
type SagaOutbox struct {
	bun.BaseModel `bun:"table:saga_outbox"`

	ID        string          `bun:"id,pk"`
	SagaID    string          `bun:"saga_id,notnull"`
	Topic     string          `bun:"topic,notnull"`
	MsgID     string          `bun:"msg_id,notnull"`
	Payload   json.RawMessage `bun:"payload,type:jsonb,notnull"`
	Status    string          `bun:"status,notnull"`
	CreatedAt time.Time       `bun:"created_at,nullzero,notnull,default:now()"`
	SentAt    *time.Time      `bun:"sent_at"`
}

// Models returns all bun models registered with the ORM.
func Models() []any {
	return []any{(*SagaInstance)(nil), (*SagaStep)(nil), (*SagaOutbox)(nil)}
}

// Saga instance states.
const (
	SagaRunning           = "RUNNING"
	SagaCompleted         = "COMPLETED"
	SagaCompensating      = "COMPENSATING"
	SagaCompensated       = "COMPENSATED"
	SagaNeedsIntervention = "NEEDS_INTERVENTION"
)

// Saga step kinds and statuses.
const (
	KindForward      = "FORWARD"
	KindCompensation = "COMPENSATION"

	StepPending   = "PENDING"
	StepSent      = "SENT"
	StepSucceeded = "SUCCEEDED"
	StepFailed    = "FAILED"
)

// Outbox row statuses.
const (
	OutboxPending = "PENDING"
	OutboxSent    = "SENT"
)

// IsTerminal reports whether a saga state needs no further driving.
func IsTerminal(state string) bool {
	switch state {
	case SagaCompleted, SagaCompensated, SagaNeedsIntervention:
		return true
	default:
		return false
	}
}
