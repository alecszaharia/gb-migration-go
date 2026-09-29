package global_blocks

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type rule struct {
	RuleType     int `json:"type"`
	AppliedFor   int
	EntityType   string
	EntityValues []string
	Mode         string
}

func (r *rule) getMode() string {
	if r.RuleType == 1 {
		return MODE_INCLUDE
	}
	return MODE_EXCLUDE
}

type newRule struct {
	project_id            int64
	global_block          int64
	mode                  string
	RuleType              string `json:"type"`
	collection_type       int64
	collection_item       int64
	collection_type_slug  string
	customer              int64
	customer_group        int64
	external_type         string
	external_id           string
	collection_type_field int64
	field_value_item      int64
}

const MODE_INCLUDE = "include"
const MODE_EXCLUDE = "exclude"

const RULE_TYPE_SPECIFIC = "specific"
const RULE_TYPE_REFERENCE = "referenced"

type ruleConverter struct {
	iriToId                   map[string]int64
	fieldIdToCollectionTypeId map[int64]int64
	repo                      *repository
}

func (rc *ruleConverter) convertOldRuleToSqlRule(r *rule) ([]newRule, error) {

	nr := newRule{mode: r.getMode()}
	hasMode := r.Mode != ""
	modeReference := hasMode && ("reference" == r.Mode)
	modeSpecific := hasMode && ("specific" == r.Mode)
	hasEntityType := r.EntityType != ""
	hasEntityValues := len(r.EntityValues) > 0
	hasEcwidItem := slices.IndexFunc(r.EntityValues, func(item string) bool { return strings.Contains(item, "ecwid-product") }) != -1
	hasEcwidCategory := slices.IndexFunc(r.EntityValues, func(item string) bool { return strings.Contains(item, "ecwid-category") }) != -1
	hasCollectionTypeFieldValue := slices.IndexFunc(r.EntityValues, func(item string) bool { return strings.Contains(item, "collection_type_fields") }) != -1
	entityTypeCustomer := hasEntityType && "customer" == r.EntityType
	noEntityValues := !hasEntityValues

	results := make([]newRule, 0)

	if !modeReference && hasEntityType && hasEntityValues && (hasEcwidItem || hasEcwidCategory) {
		for _, ev := range r.EntityValues {
			aR := nr
			parts := strings.Split(ev, "/")
			aR.RuleType = RULE_TYPE_SPECIFIC

			if parts[0] != "" {
				aR.external_type = parts[0]
			}
			if len(parts) > 1 && parts[1] != "" {
				aR.external_id = parts[1]
			}
			results = append(results, aR)
		}

		return results, nil
	}

	if modeReference && hasEntityType && hasEntityValues && hasEcwidCategory {
		for _, ev := range r.EntityValues {
			aR := nr
			values := strings.Split(ev, ":")

			var value string

			if len(values) > 1 && values[1] != "" {
				value = values[1]
			} else if values[0] != "" {
				value = values[0]
			}

			parts := strings.Split(value, "/")
			if parts[0] != "" {
				aR.RuleType = RULE_TYPE_REFERENCE
				aR.external_type = parts[0]
			}
			if len(parts) > 1 && parts[1] != "" {
				aR.RuleType = RULE_TYPE_REFERENCE
				aR.external_id = parts[1]
			}
			results = append(results, aR)
		}

		return results, nil
	}

	if modeReference && !entityTypeCustomer && hasEntityValues && hasCollectionTypeFieldValue {
		for _, ev := range r.EntityValues {
			aR := nr
			values := strings.Split(ev, ":")
			aR.RuleType = RULE_TYPE_REFERENCE

			if values[0] != "" {
				fieldId, err := rc.getIdFomIri(values[0])
				if err != nil {
					return nil, err
				}
				aR.collection_type_field = fieldId
				collectionTypeId, err := rc.repo.getCollectionTypeFromFieldTypeId(fieldId)
				if err != nil {
					return nil, fmt.Errorf("failed to get collection type for field %d: %w", fieldId, err)
				}
				aR.collection_type = collectionTypeId
			}
			if len(values) > 1 && values[1] != "" {
				id, err := rc.getIdFomIri(values[1])
				if err != nil {
					return nil, err
				}
				aR.field_value_item = id
			}

			results = append(results, aR)
		}

		return results, nil
	}

	if modeReference && !entityTypeCustomer && hasEntityValues {
		for _, ev := range r.EntityValues {
			aR := nr
			values := strings.Split(ev, ":")
			aR.RuleType = RULE_TYPE_REFERENCE
			id, err := rc.getIdFomIri(r.EntityType)
			if err != nil {
				return nil, err
			}
			aR.collection_type = id

			if len(values) > 1 && values[1] != "" {
				id, err := rc.getIdFomIri(values[1])
				if err != nil {
					return nil, err
				}
				aR.collection_item = id
			}

			results = append(results, aR)
		}

		return results, nil
	}

	if entityTypeCustomer && noEntityValues {
		aR := nr
		aR.RuleType = RULE_TYPE_REFERENCE
		aR.collection_type_slug = "customer"
		results = append(results, aR)
		return results, nil
	}

	if hasEntityType && !entityTypeCustomer && !hasEntityValues && !hasMode {
		aR := nr
		aR.RuleType = RULE_TYPE_REFERENCE
		id, err := rc.getIdFomIri(r.EntityType)
		if err != nil {
			return nil, err
		}
		aR.collection_type = id
		results = append(results, aR)
		return results, nil
	}

	if modeReference && entityTypeCustomer && hasEntityValues {

		if len(r.EntityValues) == 0 {
			return nil, fmt.Errorf("the rule must have the entityValues set")
		}

		for _, ev := range r.EntityValues {
			aR := nr
			aR.RuleType = RULE_TYPE_REFERENCE
			aR.collection_type_slug = "customer_group"
			id, err := rc.getIdFomIri(ev)
			if err != nil {
				return nil, err
			}
			aR.customer_group = id
			results = append(results, aR)
		}
		return results, nil
	}

	if (!hasMode || "reference" != r.Mode) && (r.Mode == "" || !entityTypeCustomer) && hasEntityValues {
		for _, ev := range r.EntityValues {
			aR := nr
			aR.RuleType = RULE_TYPE_SPECIFIC
			if r.EntityType != "" {
				id, err := rc.getIdFomIri(r.EntityType)
				if err != nil {
					return nil, err
				}
				aR.collection_type = id
			}
			id, err := rc.getIdFomIri(ev)
			if err != nil {
				return nil, err
			}
			aR.collection_item = id
			results = append(results, aR)
		}
		return results, nil
	}

	if hasEntityType && modeSpecific && entityTypeCustomer && hasEntityValues {
		for _, ev := range r.EntityValues {
			aR := nr
			aR.RuleType = RULE_TYPE_SPECIFIC
			aR.collection_type_slug = "customer"
			id, err := rc.getIdFomIri(ev)
			if err != nil {
				return nil, err
			}
			aR.customer = id
			results = append(results, aR)
		}
		return results, nil
	}

	if !hasEntityType && noEntityValues {
		aR := nr
		aR.RuleType = RULE_TYPE_SPECIFIC
		results = append(results, aR)
		return results, nil
	}

	return nil, fmt.Errorf("Somthing was wrong with the rule migration")
}

func (rc *ruleConverter) getIdFomIri(iri string) (int64, error) {
	if rc.iriToId == nil {
		rc.iriToId = make(map[string]int64)
	}

	if _, ok := rc.iriToId[iri]; ok {
		return rc.iriToId[iri], nil
	}

	re := regexp.MustCompile(`^/(.*)/(\d+)$`)

	matches := re.FindStringSubmatch(iri)

	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid iri %q", iri)
	}

	switch table_name := matches[1]; table_name {
	case "collection_items":
	case "collection_types":
	case "customers":
	case "customer_groups":
	case "collection_type_fields":
	default:
		return 0, fmt.Errorf("invalid iri %q: unknown table %q", iri, matches[1])
	}

	t, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid iri %q: %w", iri, err)
	}
	rc.iriToId[iri] = t
	return rc.iriToId[iri], nil
}
