package global_blocks

import (
	"BrizyGBMigration/internal/database"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	database_url string
	batch        int
	failed       bool
)

func migrationRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// db connection
	fmt.Print("Getting node and metafield ids: ")

	db, err := database.NewDB(viper.GetString("database_url"))
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
	lastId, err := migSt.getState(ctx)
	fmt.Println("starting from:", lastId)

	// initialize migration state
	latestMigrated, err := migSt.getState(ctx)

	if err != nil {
		fmt.Println("Failed to get latest migrated id")
		return fmt.Errorf("Failed to get latest migrated id: %w", err)
	}

	var count int64

	if viper.GetBool("failed") {
		count, err = repo.getFailedTotalCount(ctx)
	} else {
		count, err = repo.getTotalCount(ctx, latestMigrated)
	}

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

	return iterateThroughBatches(ctx, &migSt, repo, latestMigrated, batch, failed, count)
}

func iterateThroughBatches(ctx context.Context, migSt *state, repo *repository, latestMigrated int64, batch int, failedOnly bool, count int64) error {
	var ids []int64
	var err error
	var processed int64
	i := 1
	fmt.Println() // reserve the progress line; each batch rewrites it in place
	for {
		if failedOnly {
			ids, err = repo.getFailedGlobalBlocksIds(ctx)
		} else {
			ids, err = repo.getGlobalBlocksIds(ctx, latestMigrated, batch)
		}

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
		if err := migrateBatch(ctx, migSt, repo, ids, globalBlocks, failedOnly); err != nil {
			fmt.Println("Failed to migrate batch")
			return err
		}
		processed += int64(len(ids))
		blocksPerSecond := float64(len(ids)) / time.Since(start).Seconds()

		fmt.Printf("\033[F\033[2KProgress: %.2f%% (%d/%d) | %.2f blocks/s\n",
			float64(processed)/float64(count)*100, processed, count, blocksPerSecond)

		// exit the loop as we may end up in a infinite loop if there are broken block that cannot be migrated
		if failedOnly {
			return nil
		}

		latestMigrated = slices.Max(ids)
		i = i + 1
	}
}

func migrateBatch(ctx context.Context, migSt *state, repo *repository, ids []int64, globalBlocks []globalBlock, failedOnly bool) error {
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin batch transaction: %w", err)
	}
	// no-op after a successful Commit; releases the connection on every error path
	defer tx.Rollback()

	txRepo := repo.withTx(ctx, tx)
	txSt := migSt.withTx(ctx, tx)

	// failed ids are re-inserted below for blocks that still fail
	if failedOnly {
		if err := txSt.removeFailedIds(ctx); err != nil {
			return err
		}
	}

	failedBlocks := make(map[int64]string)

	for _, gb := range globalBlocks {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT global_block"); err != nil {
			return fmt.Errorf("failed to create savepoint for global block %d: %w", gb.id.Int64, err)
		}

		if err := migrateGlobalBlockWithRules(ctx, txRepo, gb); err != nil {
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

	if !failedOnly {
		if err := txSt.updateState(ctx, slices.Max(ids)); err != nil {
			fmt.Println("Failed to update migration state")
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch transaction: %w", err)
	}

	return nil
}

func migrateGlobalBlockWithRules(ctx context.Context, repo *repository, gb globalBlock) error {
	nid, err := migrateGlobalBlock(ctx, repo, gb)
	if err != nil {
		return err
	}

	return migrateGlobalBlockRules(ctx, repo, gb.projectId.Int64, nid, gb.rules.String)
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
		cdId,
		gb.dependencies.String,
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
		fmt.Println("Failed to insert global block")
		return 0, fmt.Errorf("failed to insert global block: %w", err)
	}

	return gbId, nil
}

func migrateGlobalBlockRules(ctx context.Context, repo *repository, projectId int64, bockId int64, rulesJson string) error {
	var data []rule

	if err := json.Unmarshal([]byte(rulesJson), &data); err != nil {
		return fmt.Errorf("failed to unmarshal the rules json: %w", err)
	}

	rc := &ruleConverter{repo: repo}

	for _, r := range data {
		rules, err := rc.convertOldRuleToSqlRule(&r)
		if err != nil {
			return fmt.Errorf("failed to convert old rule to new rule: %w", err)
		}
		for i := range rules {
			rules[i].global_block = bockId
			rules[i].project_id = projectId
			_, err := repo.insertRule(ctx, rules[i])
			if err != nil {
				return fmt.Errorf("failed to insert rule: %w", err)
			}
		}
	}
	return nil
}
