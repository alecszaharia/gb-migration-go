package global_blocks

import "testing"

func seqIDs(from, n int64) []int64 {
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = from + int64(i)
	}
	return ids
}

func TestComputeSplit(t *testing.T) {
	tests := []struct {
		name      string
		ids       []int64
		k         int
		wantSizes []int // nil: only the ±1 invariant is checked
	}{
		{name: "N=10 K=3", ids: seqIDs(1, 10), k: 3, wantSizes: []int{4, 3, 3}},
		{name: "N=K", ids: seqIDs(100, 5), k: 5, wantSizes: []int{1, 1, 1, 1, 1}},
		{name: "N<K", ids: []int64{7, 8, 9}, k: 8, wantSizes: []int{1, 1, 1}},
		{name: "K=1", ids: seqIDs(1, 6), k: 1, wantSizes: []int{6}},
		{name: "N=0 nil", ids: nil, k: 4},
		{name: "N=0 empty", ids: []int64{}, k: 1},
		{name: "sparse", ids: []int64{2, 3, 50, 51, 52, 900, 10_000, 10_001, 99_999}, k: 4, wantSizes: []int{3, 2, 2, 2}},
		{name: "gapped big jumps", ids: []int64{1, 1_000, 1_000_000, 1 << 32, 1<<32 + 5, 1 << 40, 1<<40 + 1}, k: 3, wantSizes: []int{3, 2, 2}},
		{name: "sparse N<K", ids: []int64{10, 500, 90_000}, k: 50, wantSizes: []int{1, 1, 1}},
		{name: "large", ids: seqIDs(1, 1000), k: 7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rs := computeSplit(tc.ids, tc.k)
			n := len(tc.ids)

			if n == 0 {
				if rs != nil {
					t.Fatalf("want nil for empty input, got %+v", rs)
				}
				return
			}

			// count = min(K, N)
			want := min(tc.k, n)
			if len(rs) != want {
				t.Fatalf("want %d ranges, got %d", want, len(rs))
			}

			// Every ID in exactly one range; tally per-range sizes.
			sizes := make([]int, len(rs))
			for _, id := range tc.ids {
				hits := 0
				for i, r := range rs {
					if r.contains(id) {
						hits++
						sizes[i]++
					}
				}
				if hits != 1 {
					t.Fatalf("id %d contained in %d ranges, want exactly 1", id, hits)
				}
			}

			if tc.wantSizes != nil {
				if len(sizes) != len(tc.wantSizes) {
					t.Fatalf("sizes %v, want %v", sizes, tc.wantSizes)
				}
				for i := range sizes {
					if sizes[i] != tc.wantSizes[i] {
						t.Fatalf("sizes %v, want %v", sizes, tc.wantSizes)
					}
				}
			}

			// Every size within ±1 of N/K.
			base := n / want
			for i, s := range sizes {
				if s != base && s != base+1 {
					t.Errorf("range %d size %d not within ±1 of N/K (base %d)", i, s, base)
				}
			}

			maxID := tc.ids[n-1]
			for i, r := range rs {
				if r.Index != i {
					t.Errorf("range %d has Index %d", i, r.Index)
				}

				if i == len(rs)-1 {
					// Only the last range is open-ended and owns future IDs.
					if r.Upper != nil {
						t.Fatalf("last range must have nil Upper, got %d", *r.Upper)
					}
					if !r.contains(maxID + 1000) {
						t.Errorf("last range must contain maxID+1000 (%d)", maxID+1000)
					}
					continue
				}

				if r.Upper == nil {
					t.Fatalf("non-last range %d has nil Upper", i)
				}
				if *r.Upper < r.Lower {
					t.Fatalf("range %d: Upper %d < Lower %d", i, *r.Upper, r.Lower)
				}
				if r.contains(*r.Upper + 1) {
					t.Errorf("non-last range %d contains upper+1 (%d)", i, *r.Upper+1)
				}

				// Sorted and contiguous: next lower > this upper, with no
				// input ID strictly between them.
				next := rs[i+1]
				if next.Lower <= *r.Upper {
					t.Fatalf("range %d lower %d not > range %d upper %d", i+1, next.Lower, i, *r.Upper)
				}
				for _, id := range tc.ids {
					if id > *r.Upper && id < next.Lower {
						t.Errorf("id %d falls between range %d and %d", id, i, i+1)
					}
				}
			}

			// No overlap: no range bound is contained by another range.
			for i := range rs {
				for j := range rs {
					if i == j {
						continue
					}
					if rs[j].contains(rs[i].Lower) {
						t.Errorf("range %d lower %d also contained by range %d", i, rs[i].Lower, j)
					}
					if rs[i].Upper != nil && rs[j].contains(*rs[i].Upper) {
						t.Errorf("range %d upper %d also contained by range %d", i, *rs[i].Upper, j)
					}
				}
			}
		})
	}
}

func TestComputeSplitInvalidK(t *testing.T) {
	for _, k := range []int{0, -1} {
		if got := computeSplit([]int64{1, 2, 3}, k); got != nil {
			t.Errorf("k=%d: want nil, got %+v", k, got)
		}
	}
}

func TestComputeSplitBounds(t *testing.T) {
	rs := computeSplit([]int64{1, 3, 5, 7, 9, 11, 13}, 3) // sizes 3,2,2
	if rs[0].Lower != 1 || *rs[0].Upper != 5 || rs[1].Lower != 7 || *rs[1].Upper != 9 || rs[2].Lower != 11 {
		t.Fatalf("unexpected bounds: %+v", rs)
	}
	// A gap ID inside [Lower, Upper] belongs to that range; one between
	// ranges belongs to none.
	if !rs[0].contains(4) || rs[0].contains(6) || rs[1].contains(6) {
		t.Fatalf("gap-ID membership wrong: %+v", rs)
	}
}

func TestIDRangeContains(t *testing.T) {
	u := int64(10)
	bounded := idRange{Lower: 5, Upper: &u}
	open := idRange{Lower: 5}
	tests := []struct {
		name string
		r    idRange
		id   int64
		want bool
	}{
		{"bounded below lower", bounded, 4, false},
		{"bounded at lower", bounded, 5, true},
		{"bounded at upper", bounded, 10, true},
		{"bounded above upper", bounded, 11, false},
		{"open below lower", open, 4, false},
		{"open at lower", open, 5, true},
		{"open far above", open, 1 << 62, true},
	}
	for _, tc := range tests {
		if got := tc.r.contains(tc.id); got != tc.want {
			t.Errorf("%s: contains(%d) = %v, want %v", tc.name, tc.id, got, tc.want)
		}
	}
}
