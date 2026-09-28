package global_blocks

import (
	"BrizyGBMigration/internal/database"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

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
		return fmt.Errorf("Failed to open the connection", err)
	}

	fmt.Println("OK")

	// preparing statements
	fmt.Print("Preparing statements: ")
	repo, err := newPrepareRepository(ctx, db)
	if err != nil {
		fmt.Println("Failed to prepare statements")
		return fmt.Errorf("Failed to prepare statements", err)
	}
	fmt.Println("OK")

	// initialize migration state
	fmt.Print("Preparing migration state.. ")
	migSt := state{}
	err = migSt.init(ctx, db)
	if err != nil {
		fmt.Println("Failed to initialize the state management")
		return fmt.Errorf("failed to initialize the state management", err)
	}
	lastId, err := migSt.getState(ctx)
	fmt.Println("starting from:", lastId)

	// initialize migration state
	latestMigrated, err := migSt.getState(ctx)

	if err != nil {
		fmt.Println("Failed to get latest migrated id")
		return fmt.Errorf("Failed to get latest migrated id", err)
	}

	var count int64

	if viper.GetBool("failed") {
		count, err = repo.getFailedTotalCount(ctx)
	} else {
		count, err = repo.getTotalCount(ctx, latestMigrated)
	}

	if err != nil {
		fmt.Println("Failed to get total count")
		return fmt.Errorf("failed to get total count", err)
	}

	if count == 0 {
		fmt.Println("Nothing to migrate")
		return nil
	}

	fmt.Println("Starting migration. Global blocks: ", count)
	fmt.Println("Batch size: ", batch)
	fmt.Println("Starting..")

	err = iterateThroughBatches(ctx, &migSt, repo, latestMigrated, batch, failed)

	return nil
}

func iterateThroughBatches(ctx context.Context, migSt *state, repo *repository, latestMigrated int64, batch int, failedOnly bool) error {
	var ids []int64
	var err error

	for {
		if failedOnly {
			ids, err = repo.getFailedGlobalBlocksIds(ctx)
		} else {
			ids, err = repo.getGlobalBlocksIds(ctx, latestMigrated, batch)
		}

		if err != nil {
			fmt.Println("Failed to get global block ids")
			return fmt.Errorf("failed to get global block ids", err)
		}

		if len(ids) == 0 {
			fmt.Println("No entities to process")
			return nil
		}

		globalBlocks, err := repo.getGlobalBLocks(ctx, ids)

		if err != nil {
			fmt.Println("Failed to get global block data")
			return fmt.Errorf("failed to get global block data", err)
		}

		var wg sync.WaitGroup
		var migrated = make([]int64, 0, batch)
		var failed = make([]int64, 0, batch)

		for _, gb := range globalBlocks {
			wg.Go(func() {
				id, err := migrateGlobalBlock(ctx, repo, gb)
				if err != nil {
					failed = append(failed, id)
				}

				migrateGlobalBlockRules(ctx, repo, gb.projectId.Int64, gb.id.Int64, gb.rules.String)
				migrated = append(migrated, id)
			})
		}

		wg.Wait()

		if len(migrated) > 0 {

			fmt.Printf("Migrated %d global blocks\n", len(migrated))

			latestMigrated = slices.Max(migrated)

			err := migSt.updateState(ctx, latestMigrated)
			if err != nil {
				fmt.Println("Failed to update migration state")
				return err
			}
		}

		// exit the loop as we may end up in a infinite loop if there are broken block that cannot be migrated
		if failedOnly {
			return nil
		}

		fmt.Println("Continuing migration with the next batch")

	}

	return nil
}

func migrateGlobalBlock(ctx context.Context, repo *repository, gb globalBlock) (int64, error) {

	cdId, err := repo.insertCompiledData(ctx, gb.projectId.Int64, gb.compileddataMetafieldValueId.Int64)
	if err != nil {
		fmt.Println("Failed to insert compiled data")
		return 0, fmt.Errorf("failed to insert compiled data", err)
	}

	pdId, err := repo.insertPageData(ctx, gb.projectId.Int64, gb.id.Int64)

	if err != nil {
		fmt.Println("Failed to insert page data")
		return 0, fmt.Errorf("failed to insert page data", err)
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
		return 0, fmt.Errorf("failed to insert global block", err)
	}

	return gbId, nil
}

func migrateGlobalBlockRules(ctx context.Context, repo *repository, projectId int64, bockId int64, rulesJson string) {
	var data []rule

	if err := json.Unmarshal([]byte(rulesJson), &data); err != nil {
		panic(err)
	}

	rc := &ruleConverter{repo: repo}

	for _, r := range data {
		if rules, err := rc.convertOldRuleToSqlRule(&r); err != nil {
			for i := range rules {
				rules[i].global_block = bockId
				rules[i].project_id = projectId
				_, err := repo.insertRule(ctx, rules[i])
				if err != nil {
					fmt.Println("Failed to insert rule")
					continue
				}
			}
		}
	}

}
