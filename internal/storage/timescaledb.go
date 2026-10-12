package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"gpt-load/internal/platform/config"
)

// EnsureTimescaleDB upgrades the PostgreSQL schema to a TimescaleDB hypertable
// when the operator opts in via DATABASE_TIMESCALEDB_ENABLED. It is a no-op for
// SQLite and MySQL, and when the feature is disabled. The request_logs table is
// chunked by completed_at so high-volume append-only logs get partition pruning
// and native chunk expiry. All statements are idempotent so the function is safe
// to call on every startup.
func EnsureTimescaleDB(ctx context.Context, db *gorm.DB, cfg *config.Config) error {
	if cfg == nil || !cfg.DatabaseTimescaleDBEnabled {
		return nil
	}
	if db == nil || db.Dialector == nil {
		return fmt.Errorf("ensure timescaledb: database is unavailable")
	}
	if !isPostgreSQL(db) {
		logrus.WithField("event", "timescaledb.skip").
			Warn("TimescaleDB is only supported on PostgreSQL; ignoring DATABASE_TIMESCALEDB_ENABLED")
		return nil
	}

	installed, err := timescaleDBExtensionInstalled(ctx, db)
	if err != nil {
		return fmt.Errorf("inspect timescaledb extension: %w", err)
	}
	if !installed {
		return fmt.Errorf(
			"TimescaleDB extension is not installed; install it in PostgreSQL before enabling DATABASE_TIMESCALEDB_ENABLED",
		)
	}

	if err := ensureRequestLogHypertable(ctx, db); err != nil {
		return err
	}
	if err := ensureRequestLogRetention(ctx, db, cfg.LogRetentionDays); err != nil {
		return err
	}
	logrus.WithField("event", "timescaledb.ready").
		Info("TimescaleDB hypertable configured for request_logs")
	return nil
}

func isPostgreSQL(db *gorm.DB) bool {
	switch db.Dialector.Name() {
	case "postgres", "postgresql":
		return true
	default:
		return false
	}
}

func timescaleDBExtensionInstalled(ctx context.Context, db *gorm.DB) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).
		Raw("SELECT COUNT(1) FROM pg_extension WHERE extname = 'timescaledb'").
		Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func ensureRequestLogHypertable(ctx context.Context, db *gorm.DB) error {
	already, err := requestLogIsHypertable(ctx, db)
	if err != nil {
		return err
	}
	if already {
		return nil
	}
	// create_hypertable requires the time column to be NOT NULL.
	if err := db.WithContext(ctx).
		Exec("ALTER TABLE request_logs ALTER COLUMN completed_at SET NOT NULL").Error; err != nil {
		return fmt.Errorf("set request_logs completed_at NOT NULL: %w", err)
	}
	if err := db.WithContext(ctx).
		Exec("SELECT create_hypertable('request_logs', 'completed_at', chunk_time_interval => interval '1 day', if_not_exists => true)").
		Error; err != nil {
		return fmt.Errorf("create request_logs hypertable: %w", err)
	}
	return nil
}

func requestLogIsHypertable(ctx context.Context, db *gorm.DB) (bool, error) {
	var count int64
	err := db.WithContext(ctx).
		Raw("SELECT COUNT(1) FROM timescaledb_information.hypertables WHERE hypertable_name = 'request_logs'").
		Scan(&count).Error
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func ensureRequestLogRetention(ctx context.Context, db *gorm.DB, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	// add_retention_policy is idempotent with if_not_exists; an existing policy
	// with a different interval is left untouched to avoid surprising operators.
	interval := fmt.Sprintf("%d days", retentionDays)
	if err := db.WithContext(ctx).
		Exec("SELECT add_retention_policy('request_logs', make_interval(days => $1), if_not_exists => true)", retentionDays).
		Error; err != nil {
		return fmt.Errorf("add request_logs retention policy (%s): %w", interval, err)
	}
	return nil
}
