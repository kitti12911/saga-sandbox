package config

import (
	"time"

	async "github.com/kitti12911/lib-async"
	"github.com/kitti12911/lib-monitor/profiling"
	"github.com/kitti12911/lib-monitor/tracing"
	liborm "github.com/kitti12911/lib-orm/v3"
	"github.com/kitti12911/lib-util/v3/logger"
)

// Config is the saga orchestrator configuration.
type Config struct {
	Service   Service          `mapstructure:"service"   validate:"required"`
	Logging   logger.Config    `mapstructure:"logging"`
	Tracing   tracing.Config   `mapstructure:"tracing"`
	Profiling profiling.Config `mapstructure:"profiling"`
	Database  liborm.Config    `mapstructure:"database"  validate:"required"`
	NATS      async.NATSConfig `mapstructure:"nats"      validate:"required"`
	Relay     Relay            `mapstructure:"relay"`
	Sweeper   Sweeper          `mapstructure:"sweeper"`
}

// Service holds process-level settings.
type Service struct {
	Name            string        `mapstructure:"name"             env:"SERVICE_NAME"      validate:"required"`
	Port            int           `mapstructure:"port"             env:"PORT"              validate:"required,gte=1,lte=65535"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
}

// Relay controls the outbox publishing loop.
type Relay struct {
	Interval  time.Duration `mapstructure:"interval"   env:"RELAY_INTERVAL"`
	BatchSize int           `mapstructure:"batch_size" env:"RELAY_BATCH_SIZE"`
}

// Sweeper controls the saga recovery backstop: stuck RUNNING sagas are
// re-driven; sagas exceeding MaxAttempts are escalated to NEEDS_INTERVENTION.
type Sweeper struct {
	Interval    time.Duration `mapstructure:"interval"     env:"SWEEPER_INTERVAL"`
	StuckAfter  time.Duration `mapstructure:"stuck_after"  env:"SWEEPER_STUCK_AFTER"`
	MaxAttempts int           `mapstructure:"max_attempts" env:"SWEEPER_MAX_ATTEMPTS"`
	BatchSize   int           `mapstructure:"batch_size"   env:"SWEEPER_BATCH_SIZE"`
}
