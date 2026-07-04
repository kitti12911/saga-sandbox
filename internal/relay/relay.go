// Package relay publishes pending saga_outbox rows to NATS and marks them
// sent, giving the orchestrator transactional-outbox delivery guarantees.
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	async "github.com/kitti12911/lib-async"
	orm "github.com/kitti12911/lib-orm/v4"

	"github.com/kitti12911/saga-sandbox/internal/database"
	"github.com/kitti12911/saga-sandbox/internal/messaging"
)

const (
	defaultInterval  = 2 * time.Second
	defaultBatchSize = 100
)

// Relay drains the saga outbox to the message bus.
type Relay struct {
	db        *orm.DB
	bus       *async.Bus
	interval  time.Duration
	batchSize int
}

// New builds a Relay with sane defaults applied to zero-valued options.
func New(db *orm.DB, bus *async.Bus, interval time.Duration, batchSize int) *Relay {
	if interval <= 0 {
		interval = defaultInterval
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &Relay{db: db, bus: bus, interval: interval, batchSize: batchSize}
}

// Run polls the outbox until the context is canceled.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.drain(ctx); err != nil {
				slog.ErrorContext(ctx, "saga outbox relay drain failed", "error", err)
			}
		}
	}
}

func (r *Relay) drain(ctx context.Context) error {
	var rows []database.SagaOutbox
	if err := r.db.IDB(ctx).NewSelect().
		Model(&rows).
		Where("status = ?", database.OutboxPending).
		Order("created_at ASC").
		Limit(r.batchSize).
		Scan(ctx); err != nil {
		return fmt.Errorf("select pending outbox: %w", err)
	}

	for i := range rows {
		row := rows[i]

		var cmd messaging.Command
		if err := json.Unmarshal(row.Payload, &cmd); err != nil {
			return fmt.Errorf("unmarshal outbox payload %s: %w", row.ID, err)
		}

		if err := r.bus.Publish(ctx, row.Topic, cmd); err != nil {
			return fmt.Errorf("publish outbox %s: %w", row.ID, err)
		}

		if _, err := r.db.IDB(ctx).NewUpdate().
			Model((*database.SagaOutbox)(nil)).
			Set("status = ?", database.OutboxSent).
			Set("sent_at = now()").
			Where("id = ?", row.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("mark outbox sent %s: %w", row.ID, err)
		}
	}
	return nil
}
