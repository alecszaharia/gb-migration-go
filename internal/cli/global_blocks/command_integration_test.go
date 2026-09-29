package global_blocks

// T-012: a rejected worker count must not write anything to the database
// (PE R1 c7). Skipped without GB_MIGRATION_TEST_DSN.

import (
	"context"
	"reflect"
	"testing"
)

func TestRejectedWorkerCountWritesNothing(t *testing.T) {
	fx := newFixture(t)
	fx.addEligible(1, 2, 3, 10, 11)
	fx.addFailing(failMalformedRules, 4)
	fx.addFailing(failInvalidIRI, 12)
	fx.addWrongNode(5)
	fx.addLegacyAPI(6)

	ctx := context.Background()

	// A valid run first, so the migrated tables, the failed table and the
	// state table all hold rows before the rejected runs.
	if _, err := fx.run(ctx, "-b", "3", "-w", "1"); err != nil {
		t.Fatalf("seeding run: %v", err)
	}
	// Then more unmigrated work, so a run that wrongly got past validation
	// would have something to write.
	fx.addEligible(20, 21)
	fx.addFailing(failMissingCollectionType, 22)

	before := takeSnapshot(t, fx.DB)
	if len(before.Migrated["global_block"]) == 0 || len(before.Failed) == 0 || len(before.State) == 0 {
		t.Fatalf("snapshot is trivial: %d global_block, %d failed, %d state rows",
			len(before.Migrated["global_block"]), len(before.Failed), len(before.State))
	}

	cases := []struct {
		name string
		args []string
		env  string // WORKERS env var
	}{
		{name: "flag zero", args: []string{"-w", "0"}},
		{name: "flag negative", args: []string{"-w", "-2"}},
		{name: "env zero", env: "0"},
		{name: "flag zero failed mode", args: []string{"-w", "0", "--failed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("WORKERS", tc.env)
			}
			_, err := fx.run(ctx, append([]string{"-b", "3"}, tc.args...)...)
			if err == nil {
				t.Fatal("expected an error for a rejected worker count, got nil")
			}
			if err.Error() == "" {
				t.Fatal("error message is empty")
			}

			after := takeSnapshot(t, fx.DB)
			for _, table := range migratedTables {
				if !reflect.DeepEqual(before.Migrated[table], after.Migrated[table]) {
					t.Errorf("table %s changed: %d rows before, %d after",
						table, len(before.Migrated[table]), len(after.Migrated[table]))
				}
			}
			if !reflect.DeepEqual(before.Failed, after.Failed) {
				t.Errorf("failed table changed:\n before %+v\n after  %+v", before.Failed, after.Failed)
			}
			if !reflect.DeepEqual(before.State, after.State) {
				t.Errorf("state table changed:\n before %+v\n after  %+v", before.State, after.State)
			}
		})
	}
}
