package global_blocks

// Test hooks for fault injection at named points of the migration.
//
// They are nil in production, so every call site is a no-op. Integration
// tests assign them (and restore them afterwards) to return an error, record
// progress, block on a barrier, or cancel a context at that exact point.
// Tests must assign them before the migration starts; the migration only
// reads them, so concurrent workers can call them without synchronisation.
var (
	// testHookBatchStart runs at the top of each batch of a range worker,
	// before the batch's ID lookup.
	testHookBatchStart func(rangeIdx int) error

	// testHookBeforeBatchCommit runs inside a range worker's batch
	// transaction, after the blocks and the watermark are written and before
	// the commit.
	testHookBeforeBatchCommit func(rangeIdx int) error

	// testHookSplitStoreStep runs between the inserts of the single split
	// store transaction; i counts the writes done so far.
	testHookSplitStoreStep func(i int) error

	// testHookBeforeBlock runs inside a batch transaction before each block's
	// savepoint is created. dataID is the block about to be migrated and done
	// counts the blocks of this batch already migrated or rolled back to their
	// savepoint, so done == N means "mid-batch, after N savepoints".
	testHookBeforeBlock func(dataID int64, done int) error
)

// callHookBatchStart runs testHookBatchStart when it is set.
func callHookBatchStart(rangeIdx int) error {
	if h := testHookBatchStart; h != nil {
		return h(rangeIdx)
	}
	return nil
}

// callHookBeforeBatchCommit runs testHookBeforeBatchCommit when it is set.
func callHookBeforeBatchCommit(rangeIdx int) error {
	if h := testHookBeforeBatchCommit; h != nil {
		return h(rangeIdx)
	}
	return nil
}

// callHookSplitStoreStep runs testHookSplitStoreStep when it is set.
func callHookSplitStoreStep(i int) error {
	if h := testHookSplitStoreStep; h != nil {
		return h(i)
	}
	return nil
}

// callHookBeforeBlock runs testHookBeforeBlock when it is set.
func callHookBeforeBlock(dataID int64, done int) error {
	if h := testHookBeforeBlock; h != nil {
		return h(dataID, done)
	}
	return nil
}
