package global_blocks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

// rangeKeyRe matches the per-range state keys and captures index and field.
var rangeKeyRe = regexp.MustCompile(`^range_(\d+)_(lower|upper|watermark)$`)

// storedRange is a persisted range together with its watermark: the highest
// block ID of the range whose batch has committed (Lower-1 before any batch).
type storedRange struct {
	idRange
	Watermark int64
}

type state struct {
	insertMigrationStateStm *sql.Stmt
	insertFailedIdsStm      *sql.Stmt
	removeFailedIdsStm      *sql.Stmt
	getFailedIdsStm         *sql.Stmt
	loadSplitStm            *sql.Stmt
}

// withTx returns a copy of the state whose write statements run inside tx.
func (s *state) withTx(ctx context.Context, tx *sql.Tx) *state {
	c := *s
	c.insertMigrationStateStm = tx.StmtContext(ctx, s.insertMigrationStateStm)
	c.insertFailedIdsStm = tx.StmtContext(ctx, s.insertFailedIdsStm)
	c.removeFailedIdsStm = tx.StmtContext(ctx, s.removeFailedIdsStm)
	c.getFailedIdsStm = tx.StmtContext(ctx, s.getFailedIdsStm)
	c.loadSplitStm = tx.StmtContext(ctx, s.loadSplitStm)
	return &c
}

// loadSplit reads the persisted split. ok is false when no split is stored
// (no split_k row). The legacy latest_processed_block_id key is ignored. It
// returns an error when the stored rows do not form a complete split of
// split_k ranges.
func (s *state) loadSplit(ctx context.Context) (k int, ranges []storedRange, ok bool, err error) {
	rows, err := s.loadSplitStm.QueryContext(ctx, splitKKey)
	if err != nil {
		return 0, nil, false, fmt.Errorf("failed to read the stored split: %w", err)
	}
	defer rows.Close()

	values := map[string]int64{}
	for rows.Next() {
		var key string
		var value int64
		if err := rows.Scan(&key, &value); err != nil {
			return 0, nil, false, fmt.Errorf("failed to read the stored split: %w", err)
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return 0, nil, false, fmt.Errorf("failed to read the stored split: %w", err)
	}

	return assembleSplit(values)
}

// assembleSplit builds the split from split_k and range_* state rows. Keys
// that are neither are ignored.
func assembleSplit(values map[string]int64) (int, []storedRange, bool, error) {
	kv, found := values[splitKKey]
	if !found {
		return 0, nil, false, nil
	}
	if kv < 1 {
		return 0, nil, false, fmt.Errorf("stored split is invalid: %s=%d", splitKKey, kv)
	}

	type fields struct{ lower, upper, watermark *int64 }
	byIdx := map[int]*fields{}
	for key, value := range values {
		m := rangeKeyRe.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, nil, false, fmt.Errorf("stored split has invalid key %q: %w", key, err)
		}
		f := byIdx[idx]
		if f == nil {
			f = &fields{}
			byIdx[idx] = f
		}
		v := value
		switch m[2] {
		case "lower":
			f.lower = &v
		case "upper":
			f.upper = &v
		case "watermark":
			f.watermark = &v
		}
	}

	if int64(len(byIdx)) != kv {
		return 0, nil, false, fmt.Errorf("stored split is inconsistent: %s=%d but %d ranges are stored", splitKKey, kv, len(byIdx))
	}
	k := int(kv)

	idxs := make([]int, 0, len(byIdx))
	for idx := range byIdx {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)

	ranges := make([]storedRange, 0, k)
	for i, idx := range idxs {
		if idx != i {
			return 0, nil, false, fmt.Errorf("stored split is inconsistent: %s=%d but range %d is stored", splitKKey, k, idx)
		}
		f := byIdx[idx]
		last := idx == k-1
		var missing []string
		if f.lower == nil {
			missing = append(missing, rangeLowerKey(idx))
		}
		if !last && f.upper == nil {
			missing = append(missing, rangeUpperKey(idx))
		}
		if f.watermark == nil {
			missing = append(missing, rangeWatermarkKey(idx))
		}
		if len(missing) > 0 {
			return 0, nil, false, fmt.Errorf("stored split is inconsistent: missing %s", strings.Join(missing, ", "))
		}
		if last && f.upper != nil {
			return 0, nil, false, fmt.Errorf("stored split is inconsistent: last range %d has %s", idx, rangeUpperKey(idx))
		}
		ranges = append(ranges, storedRange{
			idRange:   idRange{Index: idx, Lower: *f.lower, Upper: f.upper},
			Watermark: *f.watermark,
		})
	}

	return k, ranges, true, nil
}

// storeSplit persists split_k and every range's lower bound, upper bound
// (omitted for the open-ended last range) and initial watermark (Lower-1) in
// a single transaction, so either the whole split is stored or none of it.
// testHookSplitStoreStep runs before each write and before the commit, with
// the number of writes done so far.
func (s *state) storeSplit(ctx context.Context, db *sql.DB, ranges []idRange) (err error) {
	if err := validateSplit(ranges); err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin the split transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	txSt := s.withTx(ctx, tx)
	step := 0
	write := func(key string, value int64) error {
		if err := callHookSplitStoreStep(step); err != nil {
			return fmt.Errorf("failed to store the split: %w", err)
		}
		if _, err := txSt.insertMigrationStateStm.ExecContext(ctx, key, value, value); err != nil {
			return fmt.Errorf("failed to store %s: %w", key, err)
		}
		step++
		return nil
	}

	if err = write(splitKKey, int64(len(ranges))); err != nil {
		return err
	}
	for _, r := range ranges {
		if err = write(rangeLowerKey(r.Index), r.Lower); err != nil {
			return err
		}
		if r.Upper != nil {
			if err = write(rangeUpperKey(r.Index), *r.Upper); err != nil {
				return err
			}
		}
		if err = write(rangeWatermarkKey(r.Index), r.Lower-1); err != nil {
			return err
		}
	}

	if err = callHookSplitStoreStep(step); err != nil {
		return fmt.Errorf("failed to store the split: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return fmt.Errorf("failed to store the split: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the split: %w", err)
	}

	return nil
}

// splitStoreWrites is the number of state rows storeSplit writes for ranges:
// split_k plus lower and watermark per range plus upper for all but the last.
func splitStoreWrites(ranges []idRange) int {
	n := 1
	for _, r := range ranges {
		n += 2
		if r.Upper != nil {
			n++
		}
	}
	return n
}

// validateSplit rejects splits that loadSplit could not read back: indexes
// must be 0..n-1 in order and only the last range may (and must) be
// open-ended.
func validateSplit(ranges []idRange) error {
	if len(ranges) == 0 {
		return errors.New("cannot store an empty split")
	}
	for i, r := range ranges {
		if r.Index != i {
			return fmt.Errorf("cannot store split: range at position %d has index %d", i, r.Index)
		}
		last := i == len(ranges)-1
		if last && r.Upper != nil {
			return fmt.Errorf("cannot store split: last range %d has an upper bound", i)
		}
		if !last && r.Upper == nil {
			return fmt.Errorf("cannot store split: range %d has no upper bound", i)
		}
	}
	return nil
}

// updateRangeWatermark sets range idx's watermark to id. On a state returned
// by withTx it runs inside that transaction.
func (s *state) updateRangeWatermark(ctx context.Context, idx int, id int64) error {
	if _, err := s.insertMigrationStateStm.ExecContext(ctx, rangeWatermarkKey(idx), id, id); err != nil {
		return fmt.Errorf("failed to update the watermark of range %d: %w", idx, err)
	}
	return nil
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

// upgrade brings state tables created by earlier versions up to the current
// layout: set_key is widened from VARCHAR(25) to VARCHAR(stateKeyMaxLen) and
// value from INT to BIGINT. Existing rows are preserved; it is a no-op when
// both columns are already current, so repeated runs are safe.
func (s *state) upgrade(ctx context.Context, db *sql.DB) error {
	var (
		keyLen    sql.NullInt64
		valueType sql.NullString
	)
	err := db.QueryRowContext(ctx, `SELECT
			MAX(CASE WHEN COLUMN_NAME = 'set_key' THEN CHARACTER_MAXIMUM_LENGTH END),
			MAX(CASE WHEN COLUMN_NAME = 'value' THEN DATA_TYPE END)
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, stateTable).Scan(&keyLen, &valueType)
	if err != nil {
		return fmt.Errorf("failed to read %s column layout: %w", stateTable, err)
	}

	var mods []string
	if !keyLen.Valid || keyLen.Int64 < stateKeyMaxLen {
		mods = append(mods, fmt.Sprintf("MODIFY set_key VARCHAR(%d) NOT NULL", stateKeyMaxLen))
	}
	if !valueType.Valid || !strings.EqualFold(valueType.String, "bigint") {
		mods = append(mods, "MODIFY value BIGINT NOT NULL DEFAULT 0")
	}
	if len(mods) == 0 {
		return nil
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s %s", stateTable, strings.Join(mods, ", "))); err != nil {
		return fmt.Errorf("failed to upgrade %s: %w", stateTable, err)
	}

	return nil
}

func (s *state) init(ctx context.Context, db *sql.DB) error {
	stm, err := db.PrepareContext(ctx, `CREATE TABLE IF NOT EXISTS global_block_migration_state (
			set_key VARCHAR(64) NOT NULL PRIMARY KEY,
			value BIGINT NOT NULL DEFAULT 0
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

	// Filtered further by rangeKeyRe in assembleSplit.
	s.loadSplitStm, err = db.PrepareContext(ctx, `SELECT set_key, value FROM global_block_migration_state WHERE set_key = ? OR set_key LIKE 'range%'`)
	if err != nil {
		return fmt.Errorf("failed to to prepare statement: %s", err)
	}

	return err
}
