/*
Copyright 2026 The uwu-tools Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scan

import (
	"context"
	"fmt"
)

// Bounded limits the number of concurrent operations to a fixed size using a
// semaphore. It bounds live scans in a single process (design D9) so a burst of
// cache misses cannot spawn unbounded goroutines or overwhelm SCM rate limits. A
// broker-backed pool can later replace it behind the same Scanner seam.
type Bounded struct {
	sem chan struct{}
}

// NewBounded returns a Bounded permitting at most size concurrent operations.
// size is floored at 1.
func NewBounded(size int) *Bounded {
	if size < 1 {
		size = 1
	}
	return &Bounded{sem: make(chan struct{}, size)}
}

// Do runs fn while holding a slot, blocking until a slot is free or ctx is done.
// It returns a wrapped ctx.Err() if the context is cancelled before a slot is
// acquired; otherwise it returns fn's result.
func (b *Bounded) Do(ctx context.Context, fn func() error) error {
	select {
	case b.sem <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("scan: waiting for worker slot: %w", ctx.Err())
	}
	defer func() { <-b.sem }()
	return fn()
}

// Cap reports the maximum number of concurrent operations.
func (b *Bounded) Cap() int {
	return cap(b.sem)
}
