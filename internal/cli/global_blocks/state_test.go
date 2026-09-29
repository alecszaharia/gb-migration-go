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
