package global_blocks

import (
	"BrizyGBMigration/internal/database"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	database_url string
	batch        int
	failed       bool
	workers      int
)

// progressRenderInterval is how often the aggregate progress line is redrawn.
const progressRenderInterval = 500 * time.Millisecond

func migrationRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// db connection
	fmt.Print("Getting node and metafield ids: ")

	db, err := database.NewDB(viper.GetString("database_url"), viper.GetInt("workers"))
	if err != nil {
		fmt.Println("Failed to open the connection")
		return fmt.Errorf("Failed to open the connection: %w", err)
	}

	fmt.Println("OK")

	// preparing statements
	fmt.Print("Preparing statements: ")
	repo, err := newPrepareRepository(ctx, db)
	if err != nil {
		fmt.Println("Failed to prepare statements")
		return fmt.Errorf("Failed to prepare statements: %w", err)
	}
	fmt.Println("OK")

	// initialize migration state
	fmt.Print("Preparing migration state.. ")
	migSt := state{}
	err = migSt.init(ctx, db)
	if err != nil {
		fmt.Println("Failed to initialize the state management")
		return fmt.Errorf("failed to initialize the state management: %w", err)
	}
	fmt.Println("OK")

	// --failed is isolated from the range split and the workers: this branch
	// must stay before ensureSplit (and any loadSplit). --workers is ignored
	// here (it only sizes the connection pool), exactly one goroutine
	// processes the failed table, and migrateFailed never reads or writes any
	// migration state key (no split, no per-range watermark).
	if viper.GetBool("failed") {
		return migrateFailed(ctx, &migSt, repo)
	}

	ranges, err := ensureSplit(ctx, db, &migSt, repo, viper.GetInt("workers"))
	if err != nil {
		fmt.Println("Failed to prepare the range split")
		return err
	}
	if len(ranges) == 0 {
		return nil
	}

	fmt.Println("Getting total counts..")
	var count int64
	for _, r := range ranges {
		n, err := repo.getRemainingCountInRange(ctx, r.Watermark, r.Upper)
		if err != nil {
			fmt.Println("Failed to get total count")
			return fmt.Errorf("failed to get total count: %w", err)
		}
		count += n
	}

	if count == 0 {
		fmt.Println("Nothing to migrate")
		return nil
	}

	fmt.Println("Starting migration. Global blocks: ", count)
	fmt.Println("Batch size: ", batch)
	fmt.Println("Ranges: ", len(ranges))
	fmt.Println("Starting..")

	p := newProgress(count, nil, nil)
	p.startRendering(progressRenderInterval)
	defer p.stop()

	// One concurrent worker per range; the first fatal error cancels them all.
	if err := runRanges(ctx, &migSt, repo, ranges, batch, p); err != nil {
		// stop first: the final render rewrites the line above it
		p.stop()
		fmt.Println("Error:", err)
		return err
	}

	return nil
}

// ensureSplit returns the range split to migrate. A stored split is reused
// as-is, bounds and all, even when requestedK differs from its K. Without a
// stored split one is computed from the current eligible IDs and persisted
// before any block is migrated. When there is no stored split and nothing is
// eligible it prints "Nothing to migrate", stores nothing and returns no
// ranges.
func ensureSplit(ctx context.Context, db *sql.DB, migSt *state, repo *repository, requestedK int) ([]storedRange, error) {
	k, ranges, ok, err := migSt.loadSplit(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		if k != requestedK {
			fmt.Printf("Notice: stored split has K=%d, requested K=%d; using stored split\n", k, requestedK)
		}
		return ranges, nil
	}

	ids, err := repo.getAllEligibleIds(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		fmt.Println("Nothing to migrate")
		return nil, nil
	}

	split := computeSplit(ids, requestedK)
	if err := migSt.storeSplit(ctx, db, split); err != nil {
		return nil, err
	}

	ranges = make([]storedRange, 0, len(split))
	for _, r := range split {
		ranges = append(ranges, storedRange{idRange: r, Watermark: r.Lower - 1})
	}
	return ranges, nil
}

// runRange migrates range r batch by batch, starting after its watermark.
// Each batch is committed in its own transaction together with the range's
// new watermark (the batch maximum). It returns as soon as a lookup finds no
// more eligible IDs in the range; it never waits for new blocks. p, when not
// nil, is advanced by the size of each committed batch.
func runRange(ctx context.Context, migSt *state, repo *repository, r storedRange, batch int, p *progress) error {
	watermark := r.Watermark
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := callHookBatchStart(r.Index); err != nil {
			return err
		}

		ids, err := repo.getGlobalBlocksIdsInRange(ctx, watermark, r.Upper, batch)
		if err != nil {
			return fmt.Errorf("failed to get global block ids: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}

		globalBlocks, err := repo.getGlobalBLocks(ctx, ids)
		if err != nil {
			return fmt.Errorf("failed to get global block data: %w", err)
		}

		batchMax := slices.Max(ids)
		commitProgress := func(txSt *state) error {
			if err := txSt.updateRangeWatermark(ctx, r.Index, batchMax); err != nil {
				return err
			}
			return callHookBeforeBatchCommit(r.Index)
		}
		if err := migrateBatchWithRetry(ctx, migSt, repo, globalBlocks, commitProgress); err != nil {
			return err
		}

		watermark = batchMax
		if p != nil {
			p.add(int64(len(ids)))
		}
	}
}

// migrateFailed retries every block in the failed table in one sequential
// batch. The failed table is cleared in the batch transaction and the blocks
// that still fail are re-recorded. No migration state is read or written.
func migrateFailed(ctx context.Context, migSt *state, repo *repository) error {
	fmt.Println("Getting total counts..")
	count, err := repo.getFailedTotalCount(ctx)
	if err != nil {
		fmt.Println("Failed to get total count")
		return fmt.Errorf("failed to get total count: %w", err)
	}

	if count == 0 {
		fmt.Println("Nothing to migrate")
		return nil
	}

	fmt.Println("Starting migration. Global blocks: ", count)
	fmt.Println("Batch size: ", batch)
	fmt.Println("Starting..")
	fmt.Println() // reserve the progress line; the batch rewrites it in place

	ids, err := repo.getFailedGlobalBlocksIds(ctx)
	if err != nil {
		fmt.Println("Failed to get global block ids")
		return fmt.Errorf("failed to get global block ids: %w", err)
	}

	if len(ids) == 0 {
		fmt.Println("No entities to process")
		return nil
	}

	globalBlocks, err := repo.getGlobalBLocks(ctx, ids)
	if err != nil {
		fmt.Println("Failed to get global block data")
		return fmt.Errorf("failed to get global block data: %w", err)
	}

	start := time.Now()
	// failed ids are re-inserted by migrateBatch for blocks that still fail
	clearFailed := func(txSt *state) error { return txSt.removeFailedIds(ctx) }
	if err := migrateBatch(ctx, migSt, repo, globalBlocks, clearFailed, nil); err != nil {
		fmt.Println("Failed to migrate batch")
		return err
	}
	processed := int64(len(ids))
	blocksPerSecond := float64(processed) / time.Since(start).Seconds()

	fmt.Printf("\033[F\033[2KProgress: %.2f%% (%d/%d) | %.2f blocks/s\n",
		float64(processed)/float64(count)*100, processed, count, blocksPerSecond)

	return nil
}

// migrateBatch migrates globalBlocks in a single transaction. Each block runs
// under its own savepoint, so a failing block is rolled back and recorded in
// the failed table without affecting its batch peers. beforeBlocks, when not
// nil, runs in the transaction before any block is migrated; commitProgress,
// when not nil, runs after the blocks and failed rows are written, right
// before the commit. An error from either aborts and rolls back the batch.
// maxBatchAttempts bounds how often a batch is retried after a lock conflict.
const maxBatchAttempts = 5

// migrateBatchWithRetry runs a range batch, retrying it from scratch when the transaction
// lost a lock conflict (deadlock or lock wait timeout) with a concurrent range worker.
// A failed attempt is fully rolled back, so a retry never double-applies a block.
func migrateBatchWithRetry(ctx context.Context, migSt *state, repo *repository, globalBlocks []globalBlock, commitProgress func(txSt *state) error) error {
	var err error
	for attempt := 1; attempt <= maxBatchAttempts; attempt++ {
		err = migrateBatch(ctx, migSt, repo, globalBlocks, nil, commitProgress)
		if err == nil || !isLockConflict(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt*attempt) * 10 * time.Millisecond):
		}
	}
	return fmt.Errorf("batch failed after %d attempts: %w", maxBatchAttempts, err)
}

// isLockConflict reports whether err is a MySQL deadlock (1213) or lock wait timeout (1205).
func isLockConflict(err error) bool {
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr) && (myErr.Number == 1213 || myErr.Number == 1205)
}

func migrateBatch(ctx context.Context, migSt *state, repo *repository, globalBlocks []globalBlock, beforeBlocks, commitProgress func(txSt *state) error) error {
	// READ COMMITTED: under REPEATABLE READ a block rolled back to its savepoint leaves
	// an inherited gap lock at the end of the target tables' indexes, which deadlocks
	// concurrent range workers inserting at the same index end.
	tx, err := repo.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin batch transaction: %w", err)
	}
	// no-op after a successful Commit; releases the connection on every error path
	defer tx.Rollback()

	txRepo := repo.withTx(ctx, tx)
	txSt := migSt.withTx(ctx, tx)

	if beforeBlocks != nil {
		if err := beforeBlocks(txSt); err != nil {
			return err
		}
	}
	rc := &ruleConverter{repo: txRepo}
	failedBlocks := make(map[int64]string)

	for i, gb := range globalBlocks {
		if err := callHookBeforeBlock(gb.id.Int64, i); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "SAVEPOINT global_block"); err != nil {
			return fmt.Errorf("failed to create savepoint for global block %d: %w", gb.id.Int64, err)
		}

		if err := migrateGlobalBlockWithRules(ctx, rc, txRepo, gb); err != nil {
			// a lock conflict is not the block's fault: a deadlock has already rolled back
			// the whole transaction, so abort the batch and let the caller retry it
			if isLockConflict(err) {
				return fmt.Errorf("lock conflict migrating global block %d: %w", gb.id.Int64, err)
			}
			failedBlocks[gb.id.Int64] = err.Error()
			if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT global_block"); err != nil {
				return fmt.Errorf("failed to roll back global block %d: %w", gb.id.Int64, err)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT global_block"); err != nil {
			return fmt.Errorf("failed to release savepoint for global block %d: %w", gb.id.Int64, err)
		}
	}

	if err := txSt.addFailedIds(ctx, failedBlocks); err != nil {
		return err
	}

	if commitProgress != nil {
		if err := commitProgress(txSt); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch transaction: %w", err)
	}

	return nil
}

func migrateGlobalBlockWithRules(ctx context.Context, rc *ruleConverter, repo *repository, gb globalBlock) error {
	nid, err := migrateGlobalBlock(ctx, repo, gb)
	if err != nil {
		return err
	}

	return migrateGlobalBlockRules(ctx, rc, repo, gb.projectId.Int64, nid, gb.rules.String)
}

func migrateGlobalBlock(ctx context.Context, repo *repository, gb globalBlock) (int64, error) {

	cdId, err := repo.insertCompiledData(ctx, gb.projectId.Int64, gb.compileddataMetafieldValueId.Int64)
	if err != nil {
		fmt.Println("Failed to insert compiled data")
		return 0, fmt.Errorf("failed to insert compiled data: %w", err)
	}

	pdId, err := repo.insertPageData(ctx, gb.projectId.Int64, gb.id.Int64)

	if err != nil {
		fmt.Println("Failed to insert page data")
		return 0, fmt.Errorf("failed to insert page data: %w", err)
	}

	gbId, err := repo.insertGlobalBlock(
		ctx,
		gb.projectId.Int64,
		pdId,
		sql.NullInt64{Int64: cdId, Valid: cdId != 0},
		// dependencies is a JSON column: empty string is invalid JSON, store NULL instead
		sql.NullString{String: gb.dependencies.String, Valid: gb.dependencies.Valid && gb.dependencies.String != ""},
		gb.uid.String,
		gb.authorId.Int64,
		gb.title.String,
		gb.status.String,
		gb.meta.String,
		gb.position.String,
		gb.tags.String,
		gb.createdAt.String,
		gb.updatedAt.String,
	)

	if err != nil {
		fmt.Println("Failed to insert global block", err)
		return 0, fmt.Errorf("failed to insert global block: %w", err)
	}

	return gbId, nil
}

func migrateGlobalBlockRules(ctx context.Context, rc *ruleConverter, repo *repository, projectId int64, bockId int64, rulesJson string) error {
	var data []rule

	if err := json.Unmarshal([]byte(normalizeRuleJson(rulesJson)), &data); err != nil {
		return fmt.Errorf("failed to unmarshal the rules json: %w", err)
	}

	for _, r := range data {
		rules, err := rc.convertOldRuleToSqlRule(&r)
		if err != nil {
			fmt.Printf("failed to convert old rule to new rule: %s\n", err)
			continue
		}
		for i := range rules {
			rules[i].global_block = bockId
			rules[i].project_id = projectId
			_, err := repo.insertRule(ctx, rules[i])
			if err != nil {
				continue
			}
		}
	}
	return nil
}

func normalizeRuleJson(rulesJson string) string {
	re := regexp.MustCompile(`,"appliedFor":(.*?),`)
	return re.ReplaceAllString(rulesJson, ",")
}
