package global_blocks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type state struct {
	insertMigrationStateStm *sql.Stmt
	insertFailedIdsStm      *sql.Stmt
	removeFailedIdsStm      *sql.Stmt
	getFailedIdsStm         *sql.Stmt
}

func (s *state) getState(ctx context.Context) (int64, error) {
	var stateId int64
	row := s.getFailedIdsStm.QueryRowContext(ctx)
	if err := row.Scan(&stateId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get the state row: %s", err)
	}

	return stateId, nil
}

func (s *state) updateState(ctx context.Context, id int64) error {
	_, err := s.insertMigrationStateStm.ExecContext(ctx, "latest_processed_block_id", id, id)
	if err != nil {
		return fmt.Errorf("failed to update the migration state: %s", err)
	}

	return nil
}

func (s *state) addFailedIds(ctx context.Context, ids map[int64]string) error {
	for id, errStr := range ids {
		_, err := s.insertFailedIdsStm.ExecContext(ctx, id, errStr)
		if err != nil {
			return fmt.Errorf("failed to insert failed itd: %s", err)
		}
	}

	return nil
}
func (s *state) removeFailedIds(ctx context.Context) error {
	_, err := s.removeFailedIdsStm.ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove failed ids", err)
	}

	return nil
}

func (s *state) init(ctx context.Context, db *sql.DB) error {
	stm, err := db.PrepareContext(ctx, `CREATE TABLE IF NOT EXISTS global_block_migration_state (
			set_key VARCHAR(25) PRIMARY KEY,
			value INT NOT NULL DEFAULT 0
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}
	defer stm.Close()
	_, err = stm.ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	stm2, err := db.PrepareContext(ctx, `CREATE TABLE IF NOT EXISTS global_block_migration_failed (
			data_id INT NOT NULL PRIMARY KEY,
			error TEXT NULL,
			failed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}
	defer stm2.Close()
	_, err = stm2.ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	s.insertMigrationStateStm, err = db.PrepareContext(ctx, `INSERT INTO global_block_migration_state (set_key,value) VALUES (?,?) ON DUPLICATE KEY UPDATE value=?`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	s.insertFailedIdsStm, err = db.PrepareContext(ctx, `INSERT INTO global_block_migration_failed (data_id, error) VALUES (?,?) ON DUPLICATE KEY UPDATE error = VALUES(error), failed_at = NOW()`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}
	s.removeFailedIdsStm, err = db.PrepareContext(ctx, `DELETE FROM global_block_migration_failed WHERE 1`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	s.getFailedIdsStm, err = db.PrepareContext(ctx, `SELECT value FROM global_block_migration_state WHERE set_key='latest_processed_block_id'`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	return err
}
