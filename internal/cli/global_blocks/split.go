package global_blocks

// idRange is one contiguous slice of eligible global block IDs assigned to a
// single worker. Bounds are inclusive. Upper == nil means the range is
// open-ended: it owns every ID >= Lower, including blocks created after the
// split was computed.
type idRange struct {
	Index int
	Lower int64
	Upper *int64
}

// contains reports whether id falls inside the range. A nil Upper is treated
// as unbounded.
func (r idRange) contains(id int64) bool {
	if id < r.Lower {
		return false
	}
	return r.Upper == nil || id <= *r.Upper
}

// computeSplit partitions ascending, de-duplicated eligible IDs into
// min(k, len(ids)) contiguous, non-overlapping ranges with balanced sizes.
//
// Chunk sizes are N/K or N/K+1; the first N%K ranges receive the extra ID.
// Each range's Lower is the first ID of its chunk and Upper the last ID of its
// chunk, except the final range, whose Upper is nil (open-ended). It returns
// nil when ids is empty or k < 1.
func computeSplit(ids []int64, k int) []idRange {
	n := len(ids)
	if n == 0 || k < 1 {
		return nil
	}
	if k > n {
		k = n
	}

	base, extra := n/k, n%k
	ranges := make([]idRange, 0, k)
	start := 0
	for i := 0; i < k; i++ {
		size := base
		if i < extra {
			size++
		}
		end := start + size - 1

		r := idRange{Index: i, Lower: ids[start]}
		if i < k-1 {
			upper := ids[end]
			r.Upper = &upper
		}
		ranges = append(ranges, r)
		start = end + 1
	}

	return ranges
}
