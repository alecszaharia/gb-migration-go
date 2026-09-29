package global_blocks

import "testing"

// Every state key the tool writes must fit in set_key (RS R6 c1).
func TestStateKeysFitSetKeyColumn(t *testing.T) {
	check := func(key string) {
		t.Helper()
		if len(key) > stateKeyMaxLen {
			t.Fatalf("state key %q is %d chars, exceeds set_key width %d", key, len(key), stateKeyMaxLen)
		}
	}

	check(legacyWatermarkKey)
	check(splitKKey)
	for i := 0; i < 9999; i++ {
		check(rangeLowerKey(i))
		check(rangeUpperKey(i))
		check(rangeWatermarkKey(i))
	}
}

func TestStateKeysAreDistinct(t *testing.T) {
	seen := map[string]bool{legacyWatermarkKey: true, splitKKey: true}
	for i := 0; i < 9999; i++ {
		for _, k := range []string{rangeLowerKey(i), rangeUpperKey(i), rangeWatermarkKey(i)} {
			if seen[k] {
				t.Fatalf("duplicate state key %q", k)
			}
			seen[k] = true
		}
	}
}

func i64(v int64) *int64 { return &v }

func TestAssembleSplit(t *testing.T) {
	complete := map[string]int64{
		legacyWatermarkKey:   999,
		splitKKey:            3,
		rangeLowerKey(0):     1,
		rangeUpperKey(0):     10,
		rangeWatermarkKey(0): 5,
		rangeLowerKey(1):     11,
		rangeUpperKey(1):     20,
		rangeWatermarkKey(1): 10,
		rangeLowerKey(2):     21,
		rangeWatermarkKey(2): 20,
	}
	edit := func(set map[string]int64, drop ...string) map[string]int64 {
		m := map[string]int64{}
		for k, v := range complete {
			m[k] = v
		}
		for _, k := range drop {
			delete(m, k)
		}
		for k, v := range set {
			m[k] = v
		}
		return m
	}

	t.Run("complete", func(t *testing.T) {
		k, ranges, ok, err := assembleSplit(complete)
		if err != nil || !ok || k != 3 {
			t.Fatalf("got k=%d ok=%v err=%v", k, ok, err)
		}
		want := []storedRange{
			{idRange{0, 1, i64(10)}, 5},
			{idRange{1, 11, i64(20)}, 10},
			{idRange{2, 21, nil}, 20},
		}
		if len(ranges) != len(want) {
			t.Fatalf("got %d ranges, want %d", len(ranges), len(want))
		}
		for i, r := range ranges {
			w := want[i]
			if r.Index != w.Index || r.Lower != w.Lower || r.Watermark != w.Watermark ||
				(r.Upper == nil) != (w.Upper == nil) || (r.Upper != nil && *r.Upper != *w.Upper) {
				t.Fatalf("range %d = %+v, want %+v", i, r, w)
			}
		}
	})

	t.Run("no split", func(t *testing.T) {
		_, ranges, ok, err := assembleSplit(map[string]int64{legacyWatermarkKey: 123})
		if err != nil || ok || ranges != nil {
			t.Fatalf("got ranges=%v ok=%v err=%v", ranges, ok, err)
		}
	})

	bad := map[string]map[string]int64{
		"fewer ranges than split_k": edit(nil, rangeLowerKey(2), rangeWatermarkKey(2)),
		"more ranges than split_k":  edit(map[string]int64{rangeLowerKey(3): 31, rangeWatermarkKey(3): 30}),
		"gap in indexes": {
			splitKKey: 2, rangeLowerKey(0): 1, rangeUpperKey(0): 5, rangeWatermarkKey(0): 0,
			rangeLowerKey(2): 6, rangeWatermarkKey(2): 5,
		},
		"missing lower":        edit(nil, rangeLowerKey(1)),
		"missing watermark":    edit(nil, rangeWatermarkKey(0)),
		"missing inner upper":  edit(nil, rangeUpperKey(1)),
		"last range has upper": edit(map[string]int64{rangeUpperKey(2): 30}),
		"zero split_k":         {splitKKey: 0},
	}
	for name, values := range bad {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := assembleSplit(values); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestValidateSplit(t *testing.T) {
	if err := validateSplit(computeSplit([]int64{1, 2, 3, 4, 5, 6, 7}, 3)); err != nil {
		t.Fatalf("valid split rejected: %v", err)
	}
	cases := map[string][]idRange{
		"empty":            nil,
		"wrong index":      {{Index: 1, Lower: 1}},
		"last has upper":   {{Index: 0, Lower: 1, Upper: i64(5)}},
		"inner open-ended": {{Index: 0, Lower: 1}, {Index: 1, Lower: 6}},
	}
	for name, ranges := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateSplit(ranges); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSplitStoreWrites(t *testing.T) {
	ids := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, k := range []int{1, 3, 8} {
		// split_k + K lowers + K watermarks + K-1 uppers.
		if got, want := splitStoreWrites(computeSplit(ids, k)), 3*k; got != want {
			t.Fatalf("K=%d: got %d writes, want %d", k, got, want)
		}
	}
}
