package global_blocks

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// newTestRuleConverter returns a converter whose repository never touches the
// database: getCollectionTypeFromFieldTypeId is served from the pre-seeded cache.
func newTestRuleConverter(fieldToCollectionType map[int64]int64) *ruleConverter {
	if fieldToCollectionType == nil {
		fieldToCollectionType = map[int64]int64{}
	}
	return &ruleConverter{
		repo: &repository{
			fieldIdToCollectionTypeId: fieldToCollectionType,
			fieldIdToCollectionTypeMu: &sync.RWMutex{},
		},
	}
}

func evs(values ...string) []EntityValue {
	out := make([]EntityValue, 0, len(values))
	for _, v := range values {
		out = append(out, EntityValue(v))
	}
	return out
}

func TestRuleGetMode(t *testing.T) {
	tests := []struct {
		ruleType int
		want     string
	}{
		{ruleType: 1, want: MODE_INCLUDE},
		{ruleType: 0, want: MODE_EXCLUDE},
		{ruleType: 2, want: MODE_EXCLUDE},
		{ruleType: -1, want: MODE_EXCLUDE},
	}
	for _, tt := range tests {
		r := &rule{RuleType: tt.ruleType}
		if got := r.getMode(); got != tt.want {
			t.Errorf("RuleType=%d getMode()=%q, want %q", tt.ruleType, got, tt.want)
		}
	}
}

func TestEntityValueUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    EntityValue
		wantErr bool
	}{
		{name: "string", input: `"/collection_items/12"`, want: "/collection_items/12"},
		{name: "empty string", input: `""`, want: ""},
		{name: "integer", input: `42`, want: "42"},
		{name: "float", input: `4.5`, want: "4.5"},
		{name: "bool rejected", input: `true`, wantErr: true},
		{name: "object rejected", input: `{}`, wantErr: true},
		{name: "null", input: `null`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got EntityValue
			err := json.Unmarshal([]byte(tt.input), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Mirrors how migrateGlobalBlockRules decodes the stored JSON column, including
// legacy payloads where entityValues were stored as raw numbers. appliedFor is
// present in stored payloads but not mapped, so it must be ignored.
func TestRuleJSONDecoding(t *testing.T) {
	src := `[
		{"type":1,"appliedFor":2,"entityType":"customer","entityValues":[7,"/customers/2"],"mode":"specific"},
		{"type":0,"appliedFor":null,"entityType":"","entityValues":[],"mode":""}
	]`

	var got []rule
	if err := json.Unmarshal([]byte(src), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}

	want0 := rule{RuleType: 1, EntityType: "customer", EntityValues: evs("7", "/customers/2"), Mode: "specific"}
	if !reflect.DeepEqual(got[0], want0) {
		t.Errorf("rule[0]=%+v, want %+v", got[0], want0)
	}
	want1 := rule{RuleType: 0, EntityValues: []EntityValue{}}
	if !reflect.DeepEqual(got[1], want1) {
		t.Errorf("rule[1]=%+v, want %+v", got[1], want1)
	}
}

func TestToStringSlice(t *testing.T) {
	if got := toStringSlice(nil); len(got) != 0 {
		t.Errorf("nil input: got %v, want empty", got)
	}
	got := toStringSlice(evs("a", "", "/x/1"))
	want := []string{"a", "", "/x/1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGetIdFomIri(t *testing.T) {
	tests := []struct {
		name    string
		iri     string
		want    int64
		wantErr string
	}{
		{name: "collection_items", iri: "/collection_items/12", want: 12},
		{name: "collection_types", iri: "/collection_types/3", want: 3},
		{name: "customers", iri: "/customers/99", want: 99},
		{name: "customer_groups", iri: "/customer_groups/5", want: 5},
		{name: "collection_type_fields", iri: "/collection_type_fields/77", want: 77},
		{name: "unknown table", iri: "/pages/1", wantErr: `unknown table "pages"`},
		{name: "missing leading slash", iri: "collection_items/1", wantErr: "invalid iri"},
		{name: "missing id", iri: "/collection_items/", wantErr: "invalid iri"},
		{name: "non numeric id", iri: "/collection_items/abc", wantErr: "invalid iri"},
		{name: "trailing slash", iri: "/collection_items/1/", wantErr: "invalid iri"},
		{name: "empty", iri: "", wantErr: "invalid iri"},
		{name: "overflow", iri: "/collection_items/99999999999999999999", wantErr: "value out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := newTestRuleConverter(nil)
			got, err := rc.getIdFomIri(tt.iri)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("got id %d, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%q, want containing %q", err, tt.wantErr)
				}
				if _, cached := rc.iriToId[tt.iri]; cached {
					t.Errorf("invalid iri %q must not be cached", tt.iri)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
			if cached, ok := rc.iriToId[tt.iri]; !ok || cached != tt.want {
				t.Errorf("cache[%q]=%d (ok=%v), want %d", tt.iri, cached, ok, tt.want)
			}
		})
	}
}

func TestGetIdFomIri_CacheWins(t *testing.T) {
	rc := newTestRuleConverter(nil)
	// Cache is consulted before parsing: a pre-seeded entry short-circuits even
	// for an IRI that would otherwise be rejected.
	rc.iriToId = map[string]int64{"not-an-iri": 123}
	got, err := rc.getIdFomIri("not-an-iri")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 123 {
		t.Errorf("got %d, want 123", got)
	}
}

func TestConvertOldRuleToSqlRule(t *testing.T) {
	tests := []struct {
		name           string
		rule           rule
		fieldToColType map[int64]int64
		want           []newRule
		wantErr        string
	}{
		{
			name: "ecwid product specific, no mode",
			rule: rule{RuleType: 1, EntityType: "ecwid-product", EntityValues: evs("ecwid-product/10", "ecwid-product/11")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, external_type: "ecwid-product", external_id: "10"},
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, external_type: "ecwid-product", external_id: "11"},
			},
		},
		{
			name: "ecwid category specific, mode specific",
			rule: rule{RuleType: 0, EntityType: "ecwid-category", Mode: "specific", EntityValues: evs("ecwid-category/5")},
			want: []newRule{
				{mode: MODE_EXCLUDE, RuleType: RULE_TYPE_SPECIFIC, external_type: "ecwid-category", external_id: "5"},
			},
		},
		{
			name: "ecwid product specific without id",
			rule: rule{RuleType: 1, EntityType: "ecwid-product", EntityValues: evs("ecwid-product")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, external_type: "ecwid-product"},
			},
		},
		{
			name: "ecwid category reference with prefix",
			rule: rule{RuleType: 1, EntityType: "ecwid-product", Mode: "reference", EntityValues: evs("category:ecwid-category/7")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, external_type: "ecwid-category", external_id: "7"},
			},
		},
		{
			name: "ecwid category reference without prefix",
			rule: rule{RuleType: 1, EntityType: "ecwid-product", Mode: "reference", EntityValues: evs("ecwid-category/8")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, external_type: "ecwid-category", external_id: "8"},
			},
		},
		{
			name: "ecwid category reference with empty value after colon",
			rule: rule{RuleType: 1, EntityType: "ecwid-product", Mode: "reference", EntityValues: evs("ecwid-category/9:")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, external_type: "ecwid-category", external_id: "9"},
			},
		},
		{
			name:           "collection type field reference",
			rule:           rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_type_fields/10:/collection_items/20")},
			fieldToColType: map[int64]int64{10: 3},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_type_field: 10, field_value_item: 20},
			},
		},
		{
			name:           "collection type field reference without item",
			rule:           rule{RuleType: 0, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_type_fields/10")},
			fieldToColType: map[int64]int64{10: 3},
			want: []newRule{
				{mode: MODE_EXCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_type_field: 10},
			},
		},
		{
			name:           "collection type field reference skips invalid field iri",
			rule:           rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_type_fields/x:/collection_items/20")},
			fieldToColType: map[int64]int64{},
			want:           []newRule{},
		},
		{
			name:           "collection type field reference skips invalid item iri",
			rule:           rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_type_fields/10:/pages/20")},
			fieldToColType: map[int64]int64{10: 3},
			want:           []newRule{},
		},
		{
			name: "collection type field reference keeps valid values among invalid",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference",
				EntityValues: evs("/collection_type_fields/x:/collection_items/1", "/collection_type_fields/10:/collection_items/2", "/collection_type_fields/10:/pages/3")},
			fieldToColType: map[int64]int64{10: 3},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_type_field: 10, field_value_item: 2},
			},
		},
		{
			name: "collection item reference",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_items/5:/collection_items/9", "/collection_items/6:/collection_items/12")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_item: 9},
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_item: 12},
			},
		},
		{
			name: "collection item reference without second part",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_items/5")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3},
			},
		},
		{
			name: "collection item reference skips all values on invalid entity type",
			rule: rule{RuleType: 1, EntityType: "/pages/3", Mode: "reference", EntityValues: evs("/collection_items/5:/collection_items/9", "/collection_items/6:/collection_items/12")},
			want: []newRule{},
		},
		{
			name: "collection item reference keeps valid values among invalid",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference", EntityValues: evs("/collection_items/5:/pages/9", "/collection_items/6:/collection_items/12")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_item: 12},
			},
		},
		{
			name: "customer without values",
			rule: rule{RuleType: 1, EntityType: "customer"},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type_slug: "customer"},
			},
		},
		{
			name: "customer without values ignores mode",
			rule: rule{RuleType: 0, EntityType: "customer", Mode: "reference"},
			want: []newRule{
				{mode: MODE_EXCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type_slug: "customer"},
			},
		},
		{
			name: "collection type only, no mode",
			rule: rule{RuleType: 1, EntityType: "/collection_types/4"},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 4},
			},
		},
		{
			name: "collection type only skips invalid iri",
			rule: rule{RuleType: 1, EntityType: "collection_types/4"},
			want: []newRule{},
		},
		{
			name: "customer group reference",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "reference", EntityValues: evs("/customer_groups/1", "/customer_groups/2")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type_slug: "customer_group", customer_group: 1},
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type_slug: "customer_group", customer_group: 2},
			},
		},
		{
			// getIdFomIri does not check the table matches the target column.
			name: "customer group reference accepts any known table",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "reference", EntityValues: evs("/customers/1")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type_slug: "customer_group", customer_group: 1},
			},
		},
		{
			name: "customer group reference skips invalid iri",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "reference", EntityValues: evs("customer_groups/1")},
			want: []newRule{},
		},
		{
			name: "customer group reference keeps valid values among invalid",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "reference", EntityValues: evs("customer_groups/1", "/customer_groups/2", "/pages/3")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type_slug: "customer_group", customer_group: 2},
			},
		},
		{
			name: "specific collection items with mode",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "specific", EntityValues: evs("/collection_items/1", "/collection_items/2")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type: 3, collection_item: 1},
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type: 3, collection_item: 2},
			},
		},
		{
			name: "specific collection items without mode",
			rule: rule{RuleType: 0, EntityType: "/collection_types/3", EntityValues: evs("/collection_items/1")},
			want: []newRule{
				{mode: MODE_EXCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type: 3, collection_item: 1},
			},
		},
		{
			name: "specific collection items without entity type",
			rule: rule{RuleType: 1, EntityValues: evs("/collection_items/1")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_item: 1},
			},
		},
		{
			name: "customer with values but no mode falls into specific items branch",
			rule: rule{RuleType: 1, EntityType: "customer", EntityValues: evs("/customers/4")},
			// EntityType "customer" is not a valid IRI, so this legacy shape yields nothing.
			want: []newRule{},
		},
		{
			name: "specific collection items skips invalid item iri",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "specific", EntityValues: evs("/collection_items/x")},
			want: []newRule{},
		},
		{
			name: "specific collection items keeps valid values among invalid",
			rule: rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "specific", EntityValues: evs("/collection_items/x", "/collection_items/7", "/pages/8")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type: 3, collection_item: 7},
			},
		},
		{
			name: "specific customers",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "specific", EntityValues: evs("/customers/4", "/customers/5")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type_slug: "customer", customer: 4},
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type_slug: "customer", customer: 5},
			},
		},
		{
			name: "specific customers skips invalid iri",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "specific", EntityValues: evs("customers/4")},
			want: []newRule{},
		},
		{
			name: "specific customers keeps valid values among invalid",
			rule: rule{RuleType: 1, EntityType: "customer", Mode: "specific", EntityValues: evs("customers/4", "/customers/5")},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC, collection_type_slug: "customer", customer: 5},
			},
		},
		{
			name: "empty rule",
			rule: rule{RuleType: 1},
			want: []newRule{
				{mode: MODE_INCLUDE, RuleType: RULE_TYPE_SPECIFIC},
			},
		},
		{
			name: "empty rule with mode",
			rule: rule{RuleType: 0, Mode: "specific"},
			want: []newRule{
				{mode: MODE_EXCLUDE, RuleType: RULE_TYPE_SPECIFIC},
			},
		},
		{
			name:    "entity type with mode but no values is unsupported",
			rule:    rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "specific"},
			wantErr: "Somthing was wrong with the rule migration",
		},
		{
			name:    "entity type with reference mode but no values is unsupported",
			rule:    rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference"},
			wantErr: "Somthing was wrong with the rule migration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := newTestRuleConverter(tt.fieldToColType)
			r := tt.rule
			got, err := rc.convertOldRuleToSqlRule(&r)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("got %+v, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%q, want containing %q", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("got %+v alongside error, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// The field->collection-type lookup must be served from the repository cache;
// a miss would hit the (nil) database and panic.
func TestConvertOldRuleToSqlRule_FieldLookupUsesCache(t *testing.T) {
	rc := newTestRuleConverter(map[int64]int64{10: 3, 11: 3})
	r := rule{RuleType: 1, EntityType: "/collection_types/3", Mode: "reference",
		EntityValues: evs("/collection_type_fields/10:/collection_items/1", "/collection_type_fields/11:/collection_items/2")}

	got, err := rc.convertOldRuleToSqlRule(&r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []newRule{
		{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_type_field: 10, field_value_item: 1},
		{mode: MODE_INCLUDE, RuleType: RULE_TYPE_REFERENCE, collection_type: 3, collection_type_field: 11, field_value_item: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
