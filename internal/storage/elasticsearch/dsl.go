// Copyright 2026 The HuaTuo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elasticsearch

import (
	"encoding/json"
	"fmt"
	"regexp"

	"huatuo-bamai/internal/storage/driver"
)

var fieldNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

func validateFieldName(field string) error {
	if !fieldNamePattern.MatchString(field) {
		return driver.ErrInvalidField
	}
	return nil
}

// Plain JSON types for Elasticsearch request bodies. Using explicit structs
// instead of typedapi avoids the 22 MB typedapi vendor footprint.
type (
	termClause  map[string]any
	rangeClause map[string]any
	termsClause map[string]any

	boolQuery struct {
		Filter  []map[string]any `json:"filter,omitempty"`
		MustNot []map[string]any `json:"must_not,omitempty"`
	}

	searchRequest struct {
		Query          map[string]any   `json:"query,omitempty"`
		Sort           []map[string]any `json:"sort,omitempty"`
		Size           int              `json:"size"`
		From           int              `json:"from,omitempty"`
		TrackTotalHits bool             `json:"track_total_hits"`
	}

	countRequest struct {
		Query map[string]any `json:"query,omitempty"`
	}

	termsAgg struct {
		Field string `json:"field"`
		Size  int    `json:"size"`
	}

	termsAggBody struct {
		Terms termsAgg `json:"terms"`
	}

	valuesRequest struct {
		Size  int                     `json:"size"`
		Query map[string]any          `json:"query,omitempty"`
		Aggs  map[string]termsAggBody `json:"aggs"`
	}

	// Response types for Elasticsearch API responses.
	getResponse struct {
		Found  bool            `json:"found"`
		ID     string          `json:"_id"`
		Source json.RawMessage `json:"_source"`
	}

	searchResponse struct {
		Hits struct {
			Hits []struct {
				ID     string          `json:"_id"`
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	countResponse struct {
		Count int64 `json:"count"`
	}

	valuesResponse struct {
		Aggregations struct {
			Terms struct {
				Buckets []struct {
					Key any `json:"key"`
				} `json:"buckets"`
			} `json:"terms"`
		} `json:"aggregations"`
	}
)

func buildSearchRequest(q driver.Query) ([]byte, error) {
	if q.Limit < 0 || q.Offset < 0 {
		return nil, driver.ErrNegativePagination
	}

	query, err := buildQuery(q.Filters)
	if err != nil {
		return nil, err
	}

	size := defaultQuerySize
	if q.Limit > 0 {
		size = q.Limit
	}
	req := searchRequest{
		Query:          query,
		Size:           size,
		From:           q.Offset,
		TrackTotalHits: true,
	}
	if len(q.Sorts) > 0 {
		sorts, sortErr := buildSort(q.Sorts)
		if sortErr != nil {
			return nil, sortErr
		}
		req.Sort = sorts
	}
	return json.Marshal(req)
}

func buildCountRequest(q driver.Query) ([]byte, error) {
	if q.Limit < 0 || q.Offset < 0 {
		return nil, driver.ErrNegativePagination
	}

	query, err := buildQuery(q.Filters)
	if err != nil {
		return nil, err
	}
	return json.Marshal(countRequest{Query: query})
}

func buildValuesRequest(field string, q driver.Query, size int) ([]byte, error) {
	if err := validateFieldName(field); err != nil {
		return nil, err
	}
	if q.Limit < 0 || q.Offset < 0 {
		return nil, driver.ErrNegativePagination
	}
	if size < 0 {
		return nil, driver.ErrNegativeSize
	}

	query, err := buildQuery(q.Filters)
	if err != nil {
		return nil, err
	}
	body := valuesRequest{
		Size:  0,
		Query: query,
		Aggs:  map[string]termsAggBody{"terms": {Terms: termsAgg{Field: field, Size: size}}},
	}
	return json.Marshal(body)
}

func buildQuery(filters []driver.Filter) (map[string]any, error) {
	if len(filters) == 0 {
		return map[string]any{"match_all": map[string]any{}}, nil
	}

	var filterClauses []map[string]any
	var mustNotClauses []map[string]any

	for _, f := range filters {
		clause, negate, err := buildClause(f)
		if err != nil {
			return nil, err
		}
		if negate {
			mustNotClauses = append(mustNotClauses, clause)
		} else {
			filterClauses = append(filterClauses, clause)
		}
	}

	bq := map[string]any{}
	if len(filterClauses) > 0 {
		bq["filter"] = filterClauses
	}
	if len(mustNotClauses) > 0 {
		bq["must_not"] = mustNotClauses
	}
	return map[string]any{"bool": bq}, nil
}

func buildClause(filter driver.Filter) (map[string]any, bool, error) {
	if err := validateFieldName(filter.Field); err != nil {
		return nil, false, err
	}

	switch filter.Op {
	case driver.OpEq:
		return map[string]any{"term": map[string]any{filter.Field: driver.NormalizeValue(filter.Value)}}, false, nil
	case driver.OpNe:
		return map[string]any{"term": map[string]any{filter.Field: driver.NormalizeValue(filter.Value)}}, true, nil
	case driver.OpGt, driver.OpGte, driver.OpLt, driver.OpLte:
		clause, err := buildRangeClause(filter)
		if err != nil {
			return nil, false, err
		}
		return map[string]any{"range": map[string]any{filter.Field: clause}}, false, nil
	case driver.OpIn:
		values, err := driver.FlattenInValues(filter.Value)
		if err != nil {
			return nil, false, err
		}
		return map[string]any{"terms": map[string]any{filter.Field: values}}, false, nil
	default:
		return nil, false, fmt.Errorf("%w: %s", driver.ErrUnsupportedOp, filter.Op)
	}
}

func buildRangeClause(filter driver.Filter) (map[string]any, error) {
	normalized := driver.NormalizeValue(filter.Value)
	opKey := string(filter.Op)
	return map[string]any{opKey: normalized}, nil
}

func buildSort(sorts []driver.Sort) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(sorts))
	for _, s := range sorts {
		if err := validateFieldName(s.Field); err != nil {
			return nil, err
		}
		order := "asc"
		if s.Desc {
			order = "desc"
		}
		result = append(result, map[string]any{s.Field: map[string]any{"order": order}})
	}
	return result, nil
}
