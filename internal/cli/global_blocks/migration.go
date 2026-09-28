package global_blocks

import (
	"BrizyGBMigration/internal/database"
	"context"
	"fmt"
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

	err = iterateThroughBatches(ctx, repo, latestMigrated, batch, failed)

	return nil
}

func iterateThroughBatches(ctx context.Context, repo *repository, latestMigrated int64, batch int, failedOnly bool) error {
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
				migrated = append(migrated, id)
				if err != nil {
					failed = append(failed, id)
				}
			})
		}

		wg.Wait()

		// exit the loop as we may end up in a infinite loop if there are broken block that cannot be migrated
		if failedOnly {
			return nil
		}
	}

	return nil
}

func migrateGlobalBlock(ctx context.Context, repo *repository, gb globalBlock) (int64, error) {

	repo.insertGlobalBlock(ctx,gb.projectId,gb.)

	return 0, nil
}
