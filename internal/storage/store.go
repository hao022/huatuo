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

package storage

import (
	"context"
	"fmt"

	"huatuo-bamai/internal/storage/driver"
)

// Store is a generic, backend-agnostic CRUD wrapper over a driver.Backend.
type Store[T any] struct {
	backend driver.Backend
	mapper  driver.Mapper[T]
}

// NewFromConfig creates a Store by looking up cfg.Driver in the backend registry.
func NewFromConfig[T any](ctx context.Context, cfg *driver.Config, mapper driver.Mapper[T]) (*Store[T], error) {
	backend, err := driver.NewBackend(cfg)
	if err != nil {
		return nil, err
	}
	return NewStore(ctx, backend, mapper)
}

// NewStore creates a Store from an already-constructed backend and mapper.
func NewStore[T any](ctx context.Context, backend driver.Backend, mapper driver.Mapper[T]) (*Store[T], error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if backend == nil {
		return nil, fmt.Errorf("storage: backend is nil")
	}
	if mapper == nil {
		return nil, fmt.Errorf("storage: mapper is nil")
	}

	collection := mapper.Collection()
	if collection == "" {
		return nil, fmt.Errorf("storage: collection is empty")
	}

	for _, idx := range mapper.Indexes() {
		if idx.Field == "" {
			return nil, fmt.Errorf("%w: empty index field", driver.ErrInvalidField)
		}
	}

	if err := backend.Init(ctx, collection, mapper.Indexes()); err != nil {
		return nil, err
	}

	return &Store[T]{backend: backend, mapper: mapper}, nil
}

// Save encodes v and persists it through the backend.
func (s *Store[T]) Save(ctx context.Context, v T) error {
	fields, err := s.mapper.Fields(v)
	if err != nil {
		return err
	}
	data, err := s.mapper.Encode(v)
	if err != nil {
		return fmt.Errorf("%w: %w", driver.ErrEncodeFailed, err)
	}
	rec := driver.Record{ID: s.mapper.ID(v), Data: data, Fields: fields}
	if rec.ID == "" {
		return fmt.Errorf("%w: empty id", driver.ErrInvalidField)
	}
	return s.backend.Save(driver.WithContext(ctx), rec)
}

// Get retrieves the record with the given id and decodes it.
func (s *Store[T]) Get(ctx context.Context, id string) (T, error) {
	rec, err := s.backend.Get(driver.WithContext(ctx), id)
	if err != nil {
		var zero T
		return zero, err
	}
	return s.mapper.Decode(rec.Data)
}

// Delete removes the record with the given id.
func (s *Store[T]) Delete(ctx context.Context, id string) error {
	return s.backend.Delete(driver.WithContext(ctx), id)
}

// Query returns all records matching q, decoded into T.
func (s *Store[T]) Query(ctx context.Context, q driver.Query) ([]T, error) {
	if err := s.validateQuery(q); err != nil {
		return nil, err
	}
	records, err := s.backend.Query(driver.WithContext(ctx), q)
	if err != nil {
		return nil, err
	}
	values := make([]T, 0, len(records))
	for _, rec := range records {
		value, decodeErr := s.mapper.Decode(rec.Data)
		if decodeErr != nil {
			return nil, fmt.Errorf("%w: %w", driver.ErrDecodeFailed, decodeErr)
		}
		values = append(values, value)
	}
	return values, nil
}

// Count returns the number of records matching q.
func (s *Store[T]) Count(ctx context.Context, q driver.Query) (int64, error) {
	if err := s.validateQuery(q); err != nil {
		return 0, err
	}
	return s.backend.Count(driver.WithContext(ctx), q)
}

// Values returns distinct values of field across records matching q.
func (s *Store[T]) Values(ctx context.Context, field string, q driver.Query, size int) ([]string, error) {
	if size < 0 {
		return nil, driver.ErrNegativeSize
	}
	if err := s.validateQuery(q); err != nil {
		return nil, err
	}
	return s.backend.Values(driver.WithContext(ctx), field, q, size)
}

func (s *Store[T]) validateQuery(q driver.Query) error {
	if q.Limit < 0 || q.Offset < 0 {
		return driver.ErrNegativePagination
	}
	return nil
}
