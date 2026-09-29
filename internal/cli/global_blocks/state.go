package global_blocks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	// stateTable holds key/value migration state.
	stateTable = "global_block_migration_state"
	// stateKeyMaxLen is the width of set_key; every key the tool writes must fit.
	stateKeyMaxLen = 64

	// legacyWatermarkKey is the single-cursor watermark used before range splits.
	legacyWatermarkKey = "latest_processed_block_id"
	// splitKKey stores the number of ranges in the persisted split.
	splitKKey = "split_k"

	rangeLowerKeyFmt     = "range_%d_lower"
	rangeUpperKeyFmt     = "range_%d_upper"
	rangeWatermarkKeyFmt = "range_%d_watermark"
)

// rangeLowerKey is the state key holding the inclusive lower bound of range i.
func rangeLowerKey(i int) string { return fmt.Sprintf(rangeLowerKeyFmt, i) }

// rangeUpperKey is the state key holding the inclusive upper bound of range i.
// The open-ended last range has no row for this key.
func rangeUpperKey(i int) string { return fmt.Sprintf(rangeUpperKeyFmt, i) }

// rangeWatermarkKey is the state key holding the last processed ID of range i.
func rangeWatermarkKey(i int) string { return fmt.Sprintf(rangeWatermarkKeyFmt, i) }

type state struct {
	insertMigrationStateStm *sql.Stmt
	insertFailedIdsStm      *sql.Stmt
	removeFailedIdsStm      *sql.Stmt
	getFailedIdsStm         *sql.Stmt
}

// withTx returns a copy of the state whose write statements run inside tx.
func (s *state) withTx(ctx context.Context, tx *sql.Tx) *state {
	c := *s
	c.insertMigrationStateStm = tx.StmtContext(ctx, s.insertMigrationStateStm)
	c.insertFailedIdsStm = tx.StmtContext(ctx, s.insertFailedIdsStm)
	c.removeFailedIdsStm = tx.StmtContext(ctx, s.removeFailedIdsStm)
	return &c
}

func (s *state) getState(ctx context.Context) (int64, error) {
	var stateId int64
	row := s.getFailedIdsStm.QueryRowContext(ctx, legacyWatermarkKey)
	if err := row.Scan(&stateId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get the state row: %s", err)
	}

	return stateId, nil
}

func (s *state) updateState(ctx context.Context, id int64) error {
	_, err := s.insertMigrationStateStm.ExecContext(ctx, legacyWatermarkKey, id, id)
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
		return fmt.Errorf("failed to remove failed ids: %w", err)
	}

	return nil
}

// upgrade widens set_key on state tables created by earlier versions
// (VARCHAR(25)) to VARCHAR(stateKeyMaxLen). Existing rows are preserved; it is
// a no-op when the column is already wide enough, so repeated runs are safe.
func (s *state) upgrade(ctx context.Context, db *sql.DB) error {
	var length sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT CHARACTER_MAXIMUM_LENGTH
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'set_key'`, stateTable).Scan(&length)
	if err != nil {
		return fmt.Errorf("failed to read %s.set_key length: %w", stateTable, err)
	}
	if length.Valid && length.Int64 >= stateKeyMaxLen {
		return nil
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE %s MODIFY set_key VARCHAR(%d) NOT NULL", stateTable, stateKeyMaxLen)); err != nil {
		return fmt.Errorf("failed to widen %s.set_key: %w", stateTable, err)
	}

	return nil
}

func (s *state) init(ctx context.Context, db *sql.DB) error {
	stm, err := db.PrepareContext(ctx, `CREATE TABLE IF NOT EXISTS global_block_migration_state (
			set_key VARCHAR(64) NOT NULL PRIMARY KEY,
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

	if err := s.upgrade(ctx, db); err != nil {
		return err
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

	s.getFailedIdsStm, err = db.PrepareContext(ctx, `SELECT value FROM global_block_migration_state WHERE set_key=?`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	return err
}
