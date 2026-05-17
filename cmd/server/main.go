package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	async "github.com/kitti12911/lib-async"
	"github.com/kitti12911/lib-monitor/profiling"
	"github.com/kitti12911/lib-monitor/tracing"
	libconfig "github.com/kitti12911/lib-util/v3/config"
	"github.com/kitti12911/lib-util/v3/logger"

	"github.com/kitti12911/saga-sandbox/internal/config"
	"github.com/kitti12911/saga-sandbox/internal/database"
	"github.com/kitti12911/saga-sandbox/internal/messaging"
	"github.com/kitti12911/saga-sandbox/internal/relay"
	"github.com/kitti12911/saga-sandbox/internal/saga"
	"github.com/kitti12911/saga-sandbox/internal/server"

	"github.com/dromara/carbon/v2"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()

	carbon.SetDefault(carbon.Default{
		Layout:       carbon.RFC3339Format,
		Timezone:     carbon.Bangkok,
		WeekStartsAt: carbon.Sunday,
		Locale:       "en",
	})

	cfg, err := libconfig.Load[config.Config]("config.yml")
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", "error", err)
		return 1
	}

	if cfg.Service.ShutdownTimeout == 0 {
		cfg.Service.ShutdownTimeout = 10 * time.Second
	}

	logger.NewFromConfig(cfg.Logging, cfg.Service.Name)

	profiler, err := profiling.NewFromConfig(cfg.Service.Name, cfg.Profiling)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init profiling", "error", err)
		return 1
	}
	defer func() {
		if shutdownErr := profiling.Shutdown(profiler); shutdownErr != nil {
			slog.ErrorContext(ctx, "failed to stop profiling", "error", shutdownErr)
		}
	}()

	tp, err := tracing.NewFromConfig(ctx, cfg.Service.Name, cfg.Tracing)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init tracing", "error", err)
		return 1
	}
	defer func() {
		if shutdownErr := tracing.Shutdown(ctx, tp); shutdownErr != nil {
			slog.ErrorContext(ctx, "failed to stop tracing", "error", shutdownErr)
		}
	}()

	db, err := database.New(ctx, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init database", "error", err)
		return 1
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "failed to close database", "error", closeErr)
		}
	}()

	bus, err := async.NewNATS(cfg.NATS, nil)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to nats", "error", err)
		return 1
	}
	defer func() {
		if closeErr := bus.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "failed to close nats bus", "error", closeErr)
		}
	}()

	orchestrator := saga.New(db)
	handler := server.NewSagaHandler(orchestrator)
	rly := relay.New(db, bus, cfg.Relay.Interval, cfg.Relay.BatchSize)
	sweeper := saga.NewSweeper(
		db,
		cfg.Sweeper.Interval,
		cfg.Sweeper.StuckAfter,
		cfg.Sweeper.MaxAttempts,
		cfg.Sweeper.BatchSize,
	)

	srv, err := server.NewGRPCServer(ctx, cfg.Service.Port, handler)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create gRPC server", "error", err)
		return 1
	}

	workerCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 5)
	go func() { errs <- srv.Start() }()
	go func() { errs <- rly.Run(workerCtx) }()
	go func() { errs <- sweeper.Run(workerCtx) }()
	go func() { errs <- consumeReply(workerCtx, bus, messaging.SubjectCaptureReply, orchestrator) }()
	go func() { errs <- consumeReply(workerCtx, bus, messaging.SubjectRefundReply, orchestrator) }()

	slog.InfoContext(ctx, "saga orchestrator started", "port", cfg.Service.Port)

	select {
	case <-workerCtx.Done():
	case err := <-errs:
		if err != nil {
			slog.ErrorContext(ctx, "saga orchestrator stopped with error", "error", err)
			stop()
			shutdownGRPC(ctx, srv, cfg.Service.ShutdownTimeout)
			return 1
		}
	}

	slog.InfoContext(ctx, "shutting down saga orchestrator")
	stop()
	shutdownGRPC(ctx, srv, cfg.Service.ShutdownTimeout)

	slog.InfoContext(ctx, "saga orchestrator stopped")
	return 0
}

func shutdownGRPC(ctx context.Context, srv *server.GRPCServer, timeout time.Duration) {
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	srv.Stop(shutdownCtx)
}

func consumeReply(
	ctx context.Context,
	bus *async.Bus,
	subject string,
	orchestrator *saga.Orchestrator,
) error {
	err := async.Consume(
		ctx,
		bus.Subscriber(),
		async.JSONCodec{},
		subject,
		func(ctx context.Context, msg async.Envelope[messaging.Reply]) error {
			return orchestrator.HandleReply(ctx, msg.Payload)
		},
		async.WithErrorHandler(func(ctx context.Context, msg async.Envelope[[]byte], handlerErr error) {
			slog.ErrorContext(ctx, "failed to process saga reply",
				"subject", subject, "message_uuid", msg.UUID, "error", handlerErr)
		}),
	)
	if err != nil {
		return fmt.Errorf("consume %s: %w", subject, err)
	}
	return nil
}
