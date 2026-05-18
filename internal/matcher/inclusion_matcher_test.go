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

// --- newInclusionMatcher ---

func TestNewInclusionMatcher_NilSpecs(t *testing.T) {
	im, err := newInclusionMatcher[event](nil, nil)
	require.NoError(t, err)
	require.Nil(t, im.include)
	require.Nil(t, im.exclude)
}

func TestNewInclusionMatcher_EmptySpecs(t *testing.T) {
	im, err := newInclusionMatcher([]FieldSpec[event]{}, []FieldSpec[event]{})
	require.NoError(t, err)
	require.Nil(t, im.include)
	require.Nil(t, im.exclude)
}

func TestNewInclusionMatcher_IncludeOnly(t *testing.T) {
	im, err := newInclusionMatcher(eventSpecs("cpu", "", ""), nil)
	require.NoError(t, err)
	require.NotNil(t, im.include)
	require.Nil(t, im.exclude)
}

func TestNewInclusionMatcher_ExcludeOnly(t *testing.T) {
	im, err := newInclusionMatcher[event](nil, eventSpecs("cpu", "", ""))
	require.NoError(t, err)
	require.Nil(t, im.include)
	require.NotNil(t, im.exclude)
}

func TestNewInclusionMatcher_InvalidIncludePattern(t *testing.T) {
	_, err := newInclusionMatcher(eventSpecs("[bad", "", ""), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), `"name"`)
}

func TestNewInclusionMatcher_InvalidExcludePattern(t *testing.T) {
	_, err := newInclusionMatcher[event](nil, eventSpecs("[bad", "", ""))
	require.Error(t, err)
	require.Contains(t, err.Error(), `"name"`)
}

// --- match: no include, no exclude ---

func TestInclusionMatcher_Match_NoFilter_AlwaysTrue(t *testing.T) {
	im, _ := newInclusionMatcher[event](nil, nil)
	require.True(t, im.match(event{Name: "cpu", Host: "node-1", Region: "cn"}))
	require.True(t, im.match(event{}))
}

// --- match: include only ---

func TestInclusionMatcher_Match_IncludeOnly_Passes(t *testing.T) {
	im, _ := newInclusionMatcher(eventSpecs("^cpu$", "", ""), nil)
	require.True(t, im.match(event{Name: "cpu"}))
}

func TestInclusionMatcher_Match_IncludeOnly_Fails(t *testing.T) {
	im, _ := newInclusionMatcher(eventSpecs("^cpu$", "", ""), nil)
	require.False(t, im.match(event{Name: "mem"}))
}

// --- match: exclude only ---

func TestInclusionMatcher_Match_ExcludeOnly_Excluded(t *testing.T) {
	im, _ := newInclusionMatcher[event](nil, eventSpecs("^cpu$", "", ""))
	require.False(t, im.match(event{Name: "cpu"}))
}

func TestInclusionMatcher_Match_ExcludeOnly_NotExcluded(t *testing.T) {
	im, _ := newInclusionMatcher[event](nil, eventSpecs("^cpu$", "", ""))
	require.True(t, im.match(event{Name: "mem"}))
}

// --- match: include + exclude ---

func TestInclusionMatcher_Match_InInclude_NotInExclude(t *testing.T) {
	im, _ := newInclusionMatcher(
		eventSpecs("cpu|mem", "", ""),
		eventSpecs("^debug$", "", ""),
	)
	require.True(t, im.match(event{Name: "cpu"}))
	require.True(t, im.match(event{Name: "mem"}))
}

func TestInclusionMatcher_Match_InInclude_AlsoInExclude(t *testing.T) {
	// exclude wins over include
	im, _ := newInclusionMatcher(
		eventSpecs("cpu|mem", "", ""),
		eventSpecs("^cpu$", "", ""),
	)
	require.False(t, im.match(event{Name: "cpu"}))
	require.True(t, im.match(event{Name: "mem"}))
}

func TestInclusionMatcher_Match_NotInInclude_NotInExclude(t *testing.T) {
	im, _ := newInclusionMatcher(
		eventSpecs("^cpu$", "", ""),
		eventSpecs("^debug$", "", ""),
	)
	require.False(t, im.match(event{Name: "mem"}))
}

func TestInclusionMatcher_Match_MultiField_AllMustPass(t *testing.T) {
	im, _ := newInclusionMatcher(
		eventSpecs("cpu", "node-1", "cn"),
		nil,
	)
	require.True(t, im.match(event{Name: "cpu", Host: "node-1", Region: "cn"}))
	require.False(t, im.match(event{Name: "cpu", Host: "node-2", Region: "cn"}))
	require.False(t, im.match(event{Name: "mem", Host: "node-1", Region: "cn"}))
}
