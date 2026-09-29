package global_blocks

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sync/errgroup"
)

// runRanges migrates every range concurrently, one worker goroutine per
// range. A range with no remaining work returns as soon as its first lookup
// comes back empty.
//
// Per-block failures never reach this level: migrateBatch records them in the
// failed table. Any other worker error is fatal. It is wrapped with the range
// it happened in (see rangeError) and cancels the group context, so the other
// workers stop before their next batch; batches already in flight roll back
// because their transactions use the cancelled context. The returned error is
// always the first fatal worker error.
func runRanges(ctx context.Context, migSt *state, repo *repository, ranges []storedRange, batch int, p *progress) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, r := range ranges {
		g.Go(func() error {
			err := runRange(gctx, migSt, repo, r, batch, p)
			if err == nil {
				return nil
			}
			// Stopped because another worker failed: that worker's error is
			// already recorded by the group (errgroup stores the first error
			// before cancelling), so this cancellation is not reported.
			if ctx.Err() == nil && gctx.Err() != nil && errors.Is(err, context.Canceled) {
				return nil
			}
			return rangeError(r, err)
		})
	}
	return g.Wait()
}

// rangeError wraps err with the range's index and inclusive bounds, e.g.
// "range 2 [120, 179]: ..."; an open-ended upper bound is shown as "+inf".
func rangeError(r storedRange, err error) error {
	upper := "+inf"
	if r.Upper != nil {
		upper = strconv.FormatInt(*r.Upper, 10)
	}
	return fmt.Errorf("range %d [%d, %s]: %w", r.Index, r.Lower, upper, err)
}
