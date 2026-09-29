package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

func NewDB(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the dsn: %w", err)
	}

	// Inline query args client-side so db.Query/Exec with args costs one round trip
	// instead of prepare + execute + close. Explicitly prepared statements are unaffected.
	cfg.InterpolateParams = true

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create the connector: %w", err)
	}

	db := sql.OpenDB(connector)

	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	// OpenDB doesn't open a connection. Validate DSN data:
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping the connection: %w", err)
	}

	return db, nil
}
