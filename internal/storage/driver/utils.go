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

package driver

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// WithContext returns ctx if non-nil, otherwise context.Background().
func WithContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// NormalizeValue canonicalizes time.Time to a fixed UTC string; other types pass through.
func NormalizeValue(value any) any {
	if t, ok := value.(time.Time); ok {
		return t.UTC().Format("2006-01-02 15:04:05.000 -0700")
	}
	return value
}

// FlattenInValues reflects over a slice/array and normalizes each element.
func FlattenInValues(value any) ([]any, error) {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil, ErrInRequiresSlice
	}
	if rv.Len() == 0 {
		return nil, ErrInRequiresNonEmpty
	}
	values := make([]any, 0, rv.Len())
	for i := range rv.Len() {
		values = append(values, NormalizeValue(rv.Index(i).Interface()))
	}
	return values, nil
}

// CloneBytes returns a defensive copy of data.
func CloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}

// StringValue converts any scalar to its string representation.
func StringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
