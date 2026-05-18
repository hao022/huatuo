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

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// sample type used across all tests — keeps tests self-contained.
type event struct {
	Name   string
	Host   string
	Region string
}

func eventSpecs(namePattern, hostPattern, regionPattern string) []FieldSpec[event] {
	return []FieldSpec[event]{
		{Name: "name", Pattern: namePattern, Extract: func(e event) string { return e.Name }},
		{Name: "host", Pattern: hostPattern, Extract: func(e event) string { return e.Host }},
		{Name: "region", Pattern: regionPattern, Extract: func(e event) string { return e.Region }},
	}
}

// --- NewFieldMatcher ---

func TestNewFieldMatcher_EmptySpecs(t *testing.T) {
	fm, err := NewFieldMatcher([]FieldSpec[event]{})
	require.NoError(t, err)
	require.NotNil(t, fm)
	require.Empty(t, fm.rules)
}

func TestNewFieldMatcher_AllEmptyPatterns(t *testing.T) {
	fm, err := NewFieldMatcher(eventSpecs("", "", ""))
	require.NoError(t, err)
	require.Empty(t, fm.rules)
}

func TestNewFieldMatcher_ValidPatterns(t *testing.T) {
	fm, err := NewFieldMatcher(eventSpecs("cpu.*", "^node-[0-9]+$", ""))
	require.NoError(t, err)
	require.Len(t, fm.rules, 2)
}

func TestNewFieldMatcher_InvalidPattern(t *testing.T) {
	_, err := NewFieldMatcher(eventSpecs("[invalid", "", ""))
	require.Error(t, err)
	require.Contains(t, err.Error(), `"name"`)
	require.Contains(t, err.Error(), "[invalid")
}

func TestNewFieldMatcher_InvalidPatternSecondField(t *testing.T) {
	_, err := NewFieldMatcher(eventSpecs("cpu", "[bad", ""))
	require.Error(t, err)
	require.Contains(t, err.Error(), `"host"`)
}

// --- FieldMatcher.Match ---

func TestFieldMatcher_Match_NoRules_AlwaysTrue(t *testing.T) {
	fm, _ := NewFieldMatcher([]FieldSpec[event]{})
	require.True(t, fm.Match(event{Name: "anything"}))
}

func TestFieldMatcher_Match_SingleRuleMatch(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("^cpu$", "", ""))
	require.True(t, fm.Match(event{Name: "cpu"}))
}

func TestFieldMatcher_Match_SingleRuleNoMatch(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("^mem$", "", ""))
	require.False(t, fm.Match(event{Name: "cpu"}))
}

func TestFieldMatcher_Match_AllRulesMustPass(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("cpu", "node-1", "cn"))

	require.True(t, fm.Match(event{Name: "cpu", Host: "node-1", Region: "cn"}))
	require.False(t, fm.Match(event{Name: "cpu", Host: "node-2", Region: "cn"}))
	require.False(t, fm.Match(event{Name: "mem", Host: "node-1", Region: "cn"}))
}

func TestFieldMatcher_Match_PartialSubstringMatch(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("cpu", "", ""))
	// "cpu" as substring matches "cpu_usage"
	require.True(t, fm.Match(event{Name: "cpu_usage"}))
}

func TestFieldMatcher_Match_AnchoredPattern(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("^cpu$", "", ""))
	require.True(t, fm.Match(event{Name: "cpu"}))
	require.False(t, fm.Match(event{Name: "cpu_usage"}))
}

func TestFieldMatcher_Match_RegexAlternation(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("cpu|mem|disk", "", ""))
	require.True(t, fm.Match(event{Name: "cpu"}))
	require.True(t, fm.Match(event{Name: "mem"}))
	require.True(t, fm.Match(event{Name: "disk"}))
	require.False(t, fm.Match(event{Name: "net"}))
}

func TestFieldMatcher_Match_EmptyFieldValue(t *testing.T) {
	fm, _ := NewFieldMatcher(eventSpecs("cpu", "", ""))
	// empty Name does not match "cpu"
	require.False(t, fm.Match(event{Name: ""}))
}

func TestFieldMatcher_Match_PointerType(t *testing.T) {
	type record struct{ Kind string }

	specs := []FieldSpec[*record]{
		{Name: "kind", Pattern: "^error$", Extract: func(r *record) string { return r.Kind }},
	}
	fm, err := NewFieldMatcher(specs)
	require.NoError(t, err)
	require.True(t, fm.Match(&record{Kind: "error"}))
	require.False(t, fm.Match(&record{Kind: "info"}))
}
