package global_blocks

// Integration tests of worker concurrency and connection capacity (T-023).
// Criteria PE R2 c2; PE R3 c1, c7, c8.

import (
	"BrizyGBMigration/internal/database"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ccBarrier is a reusable-once n-party barrier with a timeout. The last
// arriver records the pool's in-use connection count and releases everyone.
type ccBarrier struct {
	mu       sync.Mutex
	n        int
	arrived  int
	released chan struct{}
	onFull   func()
}

func newCCBarrier(n int, onFull func()) *ccBarrier {
	return &ccBarrier{n: n, released: make(chan struct{}), onFull: onFull}
}

// wait blocks until n parties have arrived or timeout elapses.
func (b *ccBarrier) wait(timeout time.Duration) error {
	b.mu.Lock()
	b.arrived++
	if b.arrived == b.n {
		if b.onFull != nil {
			b.onFull()
		}
		close(b.released)
	}
	arrived := b.arrived
	b.mu.Unlock()

	select {
	case <-b.released:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("barrier timeout: %d of %d parties arrived", arrived, b.n)
	}
}

// ccStats counts hook calls per range and the maximum number of workers
// holding an open batch transaction at the same time.
type ccStats struct {
	mu          sync.Mutex
	starts      map[int]int
	commits     map[int]int
	inFlight    int
	maxInFlight int
	inUseAtFull int
	errs        []error
}

func (s *ccStats) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, err)
}

// ccRun runs every range of a K-worker split concurrently on a pool opened
// the way the command opens it (database.NewDB with K workers). Each worker's
// first testHookBeforeBatchCommit (with its batch transaction open) waits on
// a barrier of the ranges that have work.
func ccRun(t *testing.T, k int, seed func(fx *fixture), prepare func(e *orchEnv), working int) (*orchEnv, *ccStats) {
	t.Helper()
	fx := newFixture(t)
	seed(fx)

	db, err := database.NewDB(fx.DSN, k)
	if err != nil {
		t.Fatalf("database.NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// PE R2 c1 (precondition of c2): room for K transactions plus the
	// coordinator.
	if got := db.Stats().MaxOpenConnections; got < k+1 {
		t.Fatalf("MaxOpenConnections = %d, want >= %d", got, k+1)
	}

	e := newOrchEnv(t, db, k)
	if prepare != nil {
		prepare(e)
	}

	st := &ccStats{starts: map[int]int{}, commits: map[int]int{}}
	bar := newCCBarrier(working, func() { st.inUseAtFull = db.Stats().InUse })

	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		st.mu.Lock()
		st.starts[rangeIdx]++
		st.mu.Unlock()
		return nil
	})
	setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
		st.mu.Lock()
		st.commits[rangeIdx]++
		first := st.commits[rangeIdx] == 1
		st.inFlight++
		st.maxInFlight = max(st.maxInFlight, st.inFlight)
		st.mu.Unlock()
		defer func() {
			st.mu.Lock()
			st.inFlight--
			st.mu.Unlock()
		}()
		if first {
			if err := bar.wait(10 * time.Second); err != nil {
				err = fmt.Errorf("range %d: %w", rangeIdx, err)
				st.fail(err)
				return err
			}
		}
		return nil
	})

	const bound = 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	start := time.Now()
	err = runRanges(ctx, e.migSt, e.repo, e.ranges, 2, nil)
	elapsed := time.Since(start)
	for _, err := range st.errs {
		t.Error(err)
	}
	if err != nil {
		t.Fatalf("runRanges: %v", err)
	}
	// PE R3 c8: all workers returned after their empty lookups, no polling
	if ctx.Err() != nil || elapsed > bound/2 {
		t.Fatalf("runRanges took %v (ctx err %v), want well under %v", elapsed, ctx.Err(), bound)
	}

	// PE R2 c2: no worker ever waited for a connection
	if wc := db.Stats().WaitCount; wc != 0 {
		t.Errorf("pool WaitCount = %d, want 0", wc)
	}
	return e, st
}

// TestConcurrencyAllRangesHoldTxTogether: K=4 ranges that all have work run
// 4 concurrent workers that hold their batch transactions open at the same
// time without waiting on the connection pool (PE R2 c2; PE R3 c1, c8).
func TestConcurrencyAllRangesHoldTxTogether(t *testing.T) {
	const k = 4
	e, st := ccRun(t, k, func(fx *fixture) { fx.addEligibleN(16) }, nil, k)

	if st.maxInFlight != k {
		t.Errorf("max concurrent open batch transactions = %d, want %d", st.maxInFlight, k)
	}
	if st.inUseAtFull < k {
		t.Errorf("pool connections in use at the barrier = %d, want >= %d", st.inUseAtFull, k)
	}
	// 4 blocks per range, batch 2: 2 batches + 1 empty lookup each
	for i := range e.ranges {
		if st.starts[i] != 3 || st.commits[i] != 2 {
			t.Errorf("range %d: %d batch starts, %d commits; want 3, 2", i, st.starts[i], st.commits[i])
		}
	}
	assertAllMigratedOnce(t, e)
}

// TestConcurrencyRangeWithoutWork: a range with no remaining work exits after
// one empty lookup and no batch, while the other workers still run
// concurrently and every worker exits (PE R3 c1, c7, c8).
func TestConcurrencyRangeWithoutWork(t *testing.T) {
	const k, idle = 4, 1
	e, st := ccRun(t, k, func(fx *fixture) { fx.addEligibleN(16) }, func(e *orchEnv) {
		// range 1 is fully processed: watermark at its upper bound
		r := &e.ranges[idle]
		if err := e.migSt.updateRangeWatermark(context.Background(), idle, *r.Upper); err != nil {
			t.Fatalf("updateRangeWatermark: %v", err)
		}
		r.Watermark = *r.Upper
	}, k-1)

	if st.starts[idle] != 1 || st.commits[idle] != 0 {
		t.Errorf("idle range: %d batch starts, %d commits; want 1, 0", st.starts[idle], st.commits[idle])
	}
	if st.maxInFlight != k-1 {
		t.Errorf("max concurrent open batch transactions = %d, want %d", st.maxInFlight, k-1)
	}
	for i := range e.ranges {
		if i == idle {
			continue
		}
		if st.starts[i] != 3 || st.commits[i] != 2 {
			t.Errorf("range %d: %d batch starts, %d commits; want 3, 2", i, st.starts[i], st.commits[i])
		}
	}
	r := e.ranges[idle]
	for _, id := range migratedDataIDs(t, e.db) {
		if r.contains(id) {
			t.Errorf("idle range %d migrated block %d", idle, id)
		}
	}
}

// assertAllMigratedOnce checks every eligible block was migrated exactly
// once and each range's watermark reached its last block.
func assertAllMigratedOnce(t *testing.T, e *orchEnv) {
	t.Helper()
	counts := migratedCountByDataID(t, e.db)
	stm := stateMap(t, e.db)
	for i, r := range e.ranges {
		var maxID int64
		for id, n := range counts {
			if r.contains(id) {
				if n != 1 {
					t.Errorf("block %d migrated %d times, want 1", id, n)
				}
				maxID = max(maxID, id)
			}
		}
		if maxID == 0 {
			t.Errorf("range %d migrated nothing", i)
		}
		if got := stm[rangeWatermarkKey(i)]; got != maxID {
			t.Errorf("range %d watermark = %d, want %d", i, got, maxID)
		}
	}
}
