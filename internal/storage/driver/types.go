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

// Package driver defines the storage abstraction layer: configuration types,
// query DSL, the Backend interface, and the backend driver registry.
package driver

import (
	"context"
	"errors"
	"fmt"
)

// Sentinel errors returned by storage operations.
var (
	ErrNotFound      = errors.New("storage: not found")
	ErrInvalidQuery  = errors.New("storage: invalid query")
	ErrUnsupportedOp = errors.New("storage: unsupported op")
	ErrUnsupported   = errors.New("storage: unsupported")
	ErrInvalidField  = errors.New("storage: invalid field")
	ErrEncodeFailed  = errors.New("storage: encode failed")
	ErrDecodeFailed  = errors.New("storage: decode failed")

	ErrNegativePagination = fmt.Errorf("%w: limit and offset must be non-negative", ErrInvalidQuery)
	ErrNegativeSize       = fmt.Errorf("%w: size must be non-negative", ErrInvalidQuery)
	ErrInRequiresSlice    = fmt.Errorf("%w: in operator requires a slice or array value", ErrInvalidQuery)
	ErrInRequiresNonEmpty = fmt.Errorf("%w: in operator requires at least one value", ErrInvalidQuery)
)

// Config holds driver-specific configuration for creating a backend.
type Config struct {
	Driver string

	SQLiteDSN string

	LocalFilePath         string
	LocalFileRotationSize int
	LocalFileMaxRotation  int

	ESAddresses []string
	ESUsername  string
	ESPassword  string
	ESIndex     string
}

// Op is a filter comparison operator.
type Op string

const (
	OpEq  Op = "eq"
	OpNe  Op = "ne"
	OpGt  Op = "gt"
	OpGte Op = "gte"
	OpLt  Op = "lt"
	OpLte Op = "lte"
	OpIn  Op = "in"
)

// Filter restricts query results to records where Field Op Value holds.
type Filter struct {
	Field string
	Op    Op
	Value any
}

// Sort orders results by Field.
type Sort struct {
	Field string
	Desc  bool
}

// Query holds query parameters for listing records.
type Query struct {
	Filters []Filter
	Sorts   []Sort
	Limit   int
	Offset  int
}

// Record is the backend-neutral wire type for a single stored document.
type Record struct {
	ID     string
	Data   []byte
	Fields map[string]any
}

// Index declares a field that the backend should index for efficient filtering.
type Index struct {
	Field string
}

// Mapper maps domain objects of type T to backend Records and back.
type Mapper[T any] interface {
	Collection() string
	ID(entity T) string
	Encode(entity T) ([]byte, error)
	Decode(data []byte) (T, error)
	Fields(entity T) (map[string]any, error)
	Indexes() []Index
}

// Backend is the interface that all storage drivers must implement.
type Backend interface {
	Init(ctx context.Context, collection string, indexes []Index) error
	Save(ctx context.Context, rec Record) error
	Get(ctx context.Context, id string) (Record, error)
	Delete(ctx context.Context, id string) error
	Query(ctx context.Context, q Query) ([]Record, error)
	Count(ctx context.Context, q Query) (int64, error)
	Values(ctx context.Context, field string, q Query, size int) ([]string, error)
}
