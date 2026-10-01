package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// NewDB opens a MySQL pool sized for the given number of concurrent workers
// (see configurePool) and verifies connectivity with a ping.
func NewDB(dsn string, workers int) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the dsn: %w", err)
	}

	// Inline query args client-side so db.Query/Exec with args costs one round trip
	// instead of prepare + execute + close. Explicitly prepared statements are unaffected.
	cfg.InterpolateParams = true
	cfg.MultiStatements = true

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create the connector: %w", err)
	}

	db := sql.OpenDB(connector)

	configurePool(db, workers)
	// OpenDB doesn't open a connection. Validate DSN data:
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping the connection: %w", err)
	}

	return db, nil
}

// configurePool sizes the pool so K workers, each holding a transaction, plus
// the coordinator's own queries never wait on a connection: max(workers+1, 20).
func configurePool(db *sql.DB, workers int) {
	n := max(workers+1, 20)
	db.SetConnMaxLifetime(time.Minute * 10)
	db.SetMaxOpenConns(n)
	db.SetMaxIdleConns(n)
}
