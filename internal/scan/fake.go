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

	"github.com/uwu-tools/scorecard-api/internal/model"
)

// FakeScanner is a configurable Scanner for tests (design task 4.6). It counts
// calls and is safe for concurrent use, so the orchestrator's single-flight and
// coalescing behavior can be exercised without the live engine.
type FakeScanner struct {
	// ScanFunc, when set, implements Scan. When nil, Scan returns a minimal
	// successful result echoing the requested commit.
	ScanFunc func(ctx context.Context, ref model.RepoRef, commit string) (*Result, error)

	mu    sync.Mutex
	calls int
}

// Ensure FakeScanner satisfies the Scanner interface.
var _ Scanner = (*FakeScanner)(nil)

// Scan records the call and delegates to ScanFunc, or returns a default result.
func (f *FakeScanner) Scan(ctx context.Context, ref model.RepoRef, commit string) (*Result, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()

	if f.ScanFunc != nil {
		return f.ScanFunc(ctx, ref, commit)
	}
	return &Result{JSON2: []byte(`{"score":0.0}`), ResolvedCommit: commit, Complete: true}, nil
}

// Calls reports how many times Scan has been invoked.
func (f *FakeScanner) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
