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

package matcher

// ValueMatcher filters plain string values using pre-compiled FieldMatcher
// include/exclude semantics. Construct via NewValueMatcher.
//
// A nil ValueMatcher always returns true (match-all semantics).
type ValueMatcher struct {
	*inclusionMatcher[string]
}

// NewValueMatcher compiles include and exclude regex patterns into a ValueMatcher.
// An empty pattern string means no filter for that direction.
// Returns an error if any pattern is not a valid regular expression.
func NewValueMatcher(include, exclude string) (*ValueMatcher, error) {
	im, err := newInclusionMatcher(valueSpecs(include), valueSpecs(exclude))
	if err != nil {
		return nil, err
	}
	return &ValueMatcher{im}, nil
}

// Match reports whether value passes the filter: present in the include set
// (when one is configured) and absent from the exclude set.
// A nil ValueMatcher always returns true.
func (vm *ValueMatcher) Match(value string) bool {
	if vm == nil {
		return true
	}
	return vm.inclusionMatcher.match(value)
}

// valueSpecs wraps a single pattern string into a FieldSpec slice for string matching.
// Returns nil when pattern is empty, so the corresponding matcher is skipped.
func valueSpecs(pattern string) []FieldSpec[string] {
	if pattern == "" {
		return nil
	}
	return []FieldSpec[string]{{
		Name:    "value",
		Pattern: pattern,
		Extract: func(s string) string { return s },
	}}
}
