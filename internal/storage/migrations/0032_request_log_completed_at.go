package migrations

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const ID0032 = "0032_request_log_completed_at"

// Up0032 adds a TIMESTAMPTZ completion timestamp to request_logs. The existing
// completed_at_ms column is epoch-milliseconds and cannot be used directly as a
// TimescaleDB hypertable time column. The new column mirrors it and is backfilled
// so opt-in TimescaleDB hypertables (see storage.EnsureTimescaleDB) can chunk and
// expire logs efficiently. It is created by migration 0001 for fresh databases;
// this migration only fills it for databases upgraded from an earlier release.
func Up0032(db *gorm.DB) error {
	if err := ValidateRecoverable0032(db); err != nil {
		return err
	}
	if !db.Migrator().HasColumn("request_logs", "completed_at") {
		var statement string
		switch strings.ToLower(db.Dialector.Name()) {
		case "postgres", "postgresql":
			statement = "ALTER TABLE request_logs ADD COLUMN completed_at timestamptz"
		case "mysql":
			statement = "ALTER TABLE request_logs ADD COLUMN completed_at datetime(3)"
		case "sqlite":
			statement = "ALTER TABLE request_logs ADD COLUMN completed_at datetime"
		default:
			return fmt.Errorf("unsupported request_log completed_at migration driver %q", db.Dialector.Name())
		}
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("add request_log completed_at: %w", err)
		}
	}

	var backfill string
	switch strings.ToLower(db.Dialector.Name()) {
	case "postgres", "postgresql":
		backfill = "UPDATE request_logs SET completed_at = to_timestamp(completed_at_ms / 1000.0) WHERE completed_at IS NULL"
	case "mysql":
		backfill = "UPDATE request_logs SET completed_at = FROM_UNIXTIME(completed_at_ms / 1000.0) WHERE completed_at IS NULL"
	case "sqlite":
		backfill = "UPDATE request_logs SET completed_at = datetime(completed_at_ms / 1000.0, 'unixepoch') WHERE completed_at IS NULL"
	default:
		return fmt.Errorf("unsupported request_log completed_at migration driver %q", db.Dialector.Name())
	}
	if err := db.Exec(backfill).Error; err != nil {
		return fmt.Errorf("backfill request_log completed_at: %w", err)
	}
	return Validate0032(db)
}

func ValidateRecoverable0032(db *gorm.DB) error {
	switch strings.ToLower(db.Dialector.Name()) {
	case "sqlite", "mysql", "postgres", "postgresql":
	default:
		return fmt.Errorf("unsupported request_log completed_at migration driver %q", db.Dialector.Name())
	}
	if !db.Migrator().HasTable("request_logs") {
		return fmt.Errorf("request_logs table is missing")
	}
	if !db.Migrator().HasColumn("request_logs", "completed_at") {
		return nil
	}
	return Validate0032(db)
}

func Validate0032(db *gorm.DB) error {
	columns, err := db.Migrator().ColumnTypes("request_logs")
	if err != nil {
		return err
	}
	for _, column := range columns {
		if column.Name() != "completed_at" {
			continue
		}
		if strings.ToLower(db.Dialector.Name()) == "postgres" ||
			strings.ToLower(db.Dialector.Name()) == "postgresql" {
			if !strings.Contains(strings.ToLower(column.DatabaseTypeName()), "timestamp") &&
				!strings.Contains(strings.ToLower(column.DatabaseTypeName()), "timestamptz") {
				return fmt.Errorf("request_log completed_at must be a timestamp on PostgreSQL")
			}
		}
		return nil
	}
	return fmt.Errorf("request_log completed_at is missing")
}
