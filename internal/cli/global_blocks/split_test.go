package global_blocks

import "testing"

// Minimal sanity check; exhaustive table tests are added by T-007.
func TestComputeSplitSanity(t *testing.T) {
	if got := computeSplit(nil, 4); got != nil {
		t.Fatalf("empty input: want nil, got %v", got)
	}

	ids := []int64{1, 3, 5, 7, 9, 11, 13}
	for k := 1; k <= 10; k++ {
		rs := computeSplit(ids, k)
		want := min(k, len(ids))
		if len(rs) != want {
			t.Fatalf("k=%d: want %d ranges, got %d", k, want, len(rs))
		}
		if rs[len(rs)-1].Upper != nil {
			t.Fatalf("k=%d: last range must be open-ended", k)
		}
		base := len(ids) / want
		for i, r := range rs {
			if r.Index != i {
				t.Fatalf("k=%d: range %d has Index %d", k, i, r.Index)
			}
			size := 0
			for _, id := range ids {
				if r.contains(id) {
					size++
				}
			}
			if size != base && size != base+1 {
				t.Fatalf("k=%d range %d: size %d not in {%d,%d}", k, i, size, base, base+1)
			}
		}
		for _, id := range ids {
			n := 0
			for _, r := range rs {
				if r.contains(id) {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("k=%d: id %d in %d ranges", k, id, n)
			}
		}
	}

	rs := computeSplit(ids, 3) // sizes 3,2,2
	if rs[0].Lower != 1 || *rs[0].Upper != 5 || rs[1].Lower != 7 || *rs[1].Upper != 9 || rs[2].Lower != 11 {
		t.Fatalf("unexpected bounds: %+v", rs)
	}
	if !rs[2].contains(1 << 40) {
		t.Fatal("open-ended last range must contain IDs beyond split-time max")
	}
}
