// Package notify provides PostgreSQL LISTEN/NOTIFY helpers used for multi-instance
// coordination. A dedicated pgx connection is used for LISTEN because the GORM
// connection pool does not expose pgx notification waiting through database/sql.
package notify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"gpt-load/internal/control"
	"gpt-load/internal/platform/config"
	"gpt-load/internal/state"
)

// Coordinator wires multi-instance state propagation for PostgreSQL-backed
// deployments. When enabled, local control-plane publications broadcast a NOTIFY,
// and a dedicated listener reloads the runtime state of every other instance
// from the shared database. It is a no-op for SQLite, MySQL, or when disabled.
type Coordinator struct {
	cancel context.CancelFunc
}

// Stop terminates the listener goroutine.
func (c *Coordinator) Stop() {
	if c != nil && c.cancel != nil {
		c.cancel()
	}
}

// NewCoordinator installs the NOTIFY broadcaster on manager and starts the
// LISTEN loop. The returned Coordinator is non-nil in every case; construction
// errors only occur for misconfiguration (PostgreSQL required but unavailable).
func NewCoordinator(
	cfg *config.Config,
	db *gorm.DB,
	manager *state.Manager,
	service *control.Service,
) (*Coordinator, error) {
	if cfg == nil || !cfg.DatabaseNotifyEnabled {
		return &Coordinator{}, nil
	}
	if db == nil || manager == nil || service == nil {
		return nil, fmt.Errorf("state notify coordinator requires database, manager, and control service")
	}
	if cfg.DatabaseMetadata.Driver != config.DatabaseDriverPostgreSQL {
		logrus.WithField("event", "notify.skip").
			Warn("DATABASE_NOTIFY_ENABLED is only supported on PostgreSQL; ignoring")
		return &Coordinator{}, nil
	}

	instanceID := newInstanceID()
	change := Change{InstanceID: instanceID, Kind: "control"}
	manager.SetNotifier(func() error {
		return Broadcast(context.Background(), db, StateChannel, change)
	})

	ctx, cancel := context.WithCancel(context.Background())
	coordinator := &Coordinator{cancel: cancel}
	go func() {
		err := Listen(ctx, cfg.DatabaseDSN, StateChannel, func(received Change) {
			if received.InstanceID == instanceID {
				return
			}
			if reloadErr := service.ReloadFromDatabase(context.Background()); reloadErr != nil {
				logrus.WithError(reloadErr).
					WithField("event", "state.reload_notify").
					Warn("reload control state from notification failed")
			}
		})
		if err != nil && ctx.Err() == nil {
			logrus.WithError(err).
				WithField("event", "state.notify_listener").
				Warn("state notify listener exited")
		}
	}()
	return coordinator, nil
}

func newInstanceID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buffer)
}

// StateChannel is the PostgreSQL channel used to announce committed control-plane
// changes so other instances can reload from the shared database.
const StateChannel = "gptload_state_change"

// Change bundles the metadata carried by a NOTIFY payload. InstanceID lets a
// publisher ignore its own notification and avoid a redundant local reload.
type Change struct {
	InstanceID string `json:"instance_id"`
	Kind       string `json:"kind"`
}

// Broadcast notifies all listeners on channel with a JSON-encoded change payload.
// It is safe to call on non-PostgreSQL databases; the caller should only enable
// it for PostgreSQL.
func Broadcast(ctx context.Context, db *gorm.DB, channel string, change Change) error {
	payload, err := json.Marshal(change)
	if err != nil {
		return fmt.Errorf("encode notify payload: %w", err)
	}
	if err := db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", channel, string(payload)).Error; err != nil {
		return fmt.Errorf("notify %s: %w", channel, err)
	}
	return nil
}

// Listen opens a dedicated pgx connection to dsn, subscribes to channel, and
// invokes onNotify for every notification received until ctx is cancelled. It is
// intended to run as a long-lived goroutine. Only PostgreSQL is supported.
func Listen(
	ctx context.Context,
	dsn string,
	channel string,
	onNotify func(Change),
) error {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse notify DSN: %w", err)
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("connect notify listener: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return fmt.Errorf("listen %s: %w", channel, err)
	}

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			return fmt.Errorf("wait for notification: %w", err)
		}
		if notification == nil || onNotify == nil {
			continue
		}
		var change Change
		if decodeErr := json.Unmarshal([]byte(notification.Payload), &change); decodeErr != nil {
			continue
		}
		onNotify(change)
	}
}
