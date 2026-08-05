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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedCap(t *testing.T) {
	t.Parallel()

	if got := NewBounded(0).Cap(); got != 1 {
		t.Errorf("NewBounded(0).Cap() = %d, want 1 (floored)", got)
	}
	if got := NewBounded(5).Cap(); got != 5 {
		t.Errorf("NewBounded(5).Cap() = %d, want 5", got)
	}
}

func TestBoundedLimitsConcurrency(t *testing.T) {
	t.Parallel()

	const (
		size = 3
		jobs = 30
	)
	b := NewBounded(size)

	var current, maxObserved atomic.Int64
	var wg sync.WaitGroup
	for range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.Do(context.Background(), func() error {
				n := current.Add(1)
				for {
					m := maxObserved.Load()
					if n <= m || maxObserved.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				current.Add(-1)
				return nil
			}); err != nil {
				t.Errorf("Do: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := maxObserved.Load(); got > size {
		t.Errorf("observed concurrency %d exceeds cap %d", got, size)
	}
	if maxObserved.Load() == 0 {
		t.Error("no work ran")
	}
}

func TestBoundedContextCancelled(t *testing.T) {
	t.Parallel()

	b := NewBounded(1)
	started := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := b.Do(context.Background(), func() error {
			close(started)
			<-release
			return nil
		}); err != nil {
			t.Errorf("holder Do: %v", err)
		}
	}()

	<-started // the single slot is now held

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran atomic.Bool
	err := b.Do(ctx, func() error {
		ran.Store(true)
		return nil
	})
	if err == nil {
		t.Fatal("Do(cancelled ctx) = nil, want error")
	}
	if ran.Load() {
		t.Error("fn ran despite cancelled context")
	}

	close(release)
	wg.Wait()
}
