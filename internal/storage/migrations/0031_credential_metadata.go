package migrations

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const ID0031 = "0031_credential_metadata"

// Up0031 为 credentials 增加可空 metadata 列，承载与已加密 data 分离的、可扩展的
// 半结构化扩展数据。PostgreSQL 使用 jsonb，MySQL 使用 json，SQLite 使用 json。
func Up0031(db *gorm.DB) error {
	if err := ValidateRecoverable0031(db); err != nil {
		return err
	}
	if !db.Migrator().HasColumn("credentials", "metadata") {
		var columnType string
		switch strings.ToLower(db.Dialector.Name()) {
		case "postgres", "postgresql":
			columnType = "jsonb"
		case "mysql":
			columnType = "json"
		case "sqlite":
			columnType = "json"
		default:
			return fmt.Errorf("unsupported credential metadata migration driver %q", db.Dialector.Name())
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE credentials ADD COLUMN metadata %s NULL", columnType)).Error; err != nil {
			return fmt.Errorf("add credential metadata: %w", err)
		}
	}
	return Validate0031(db)
}

func ValidateRecoverable0031(db *gorm.DB) error {
	switch strings.ToLower(db.Dialector.Name()) {
	case "sqlite", "mysql", "postgres", "postgresql":
	default:
		return fmt.Errorf("unsupported credential metadata migration driver %q", db.Dialector.Name())
	}
	if !db.Migrator().HasTable("credentials") {
		return fmt.Errorf("credentials table is missing")
	}
	if !db.Migrator().HasColumn("credentials", "metadata") {
		return nil
	}
	return Validate0031(db)
}

func Validate0031(db *gorm.DB) error {
	columns, err := db.Migrator().ColumnTypes("credentials")
	if err != nil {
		return err
	}
	for _, column := range columns {
		if column.Name() != "metadata" {
			continue
		}
		if nullable, known := column.Nullable(); !known || !nullable {
			return fmt.Errorf("credential metadata must be nullable")
		}
		// SQLite stores JSON with TEXT affinity, so only assert the JSON type on
		// drivers that honor it.
		if strings.ToLower(db.Dialector.Name()) != "sqlite" {
			if !strings.Contains(strings.ToLower(column.DatabaseTypeName()), "json") {
				return fmt.Errorf("credential metadata must be a JSON type")
			}
		}
		return nil
	}
	return fmt.Errorf("credential metadata is missing")
}
