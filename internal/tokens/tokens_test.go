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

package tokens

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPATPoolRoundRobin(t *testing.T) {
	t.Parallel()

	p := NewPATPool([]string{"a", " b ", "", "c"})
	if got := p.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3 (blank dropped, whitespace trimmed)", got)
	}
	want := []string{"a", "b", "c", "a", "b", "c", "a"}
	for i, w := range want {
		if got := p.Next(); got != w {
			t.Errorf("Next() call %d = %q, want %q", i, got, w)
		}
	}
	if got := p.Joined(); got != "a,b,c" {
		t.Errorf("Joined() = %q, want a,b,c", got)
	}
}

func TestPATPoolEmpty(t *testing.T) {
	t.Parallel()

	p := NewPATPool(nil)
	if p.Len() != 0 {
		t.Errorf("Len() = %d, want 0", p.Len())
	}
	if got := p.Next(); got != "" {
		t.Errorf("Next() = %q, want empty", got)
	}
	if got := p.Joined(); got != "" {
		t.Errorf("Joined() = %q, want empty", got)
	}
}

func TestHostLimiterAllows(t *testing.T) {
	t.Parallel()

	h := NewHostLimiter(1000, 1)
	if err := h.Wait(context.Background(), "github.com"); err != nil {
		t.Fatalf("Wait() = %v, want nil", err)
	}
	// A different host has an independent bucket, so it is not throttled.
	if err := h.Wait(context.Background(), "gitlab.com"); err != nil {
		t.Fatalf("Wait(other host) = %v, want nil", err)
	}
}

func TestHostLimiterUnlimited(t *testing.T) {
	t.Parallel()

	h := NewHostLimiter(0, 0) // non-positive rate => unlimited, burst floored to 1
	for range 5 {
		if err := h.Wait(context.Background(), "github.com"); err != nil {
			t.Fatalf("Wait() = %v, want nil under unlimited rate", err)
		}
	}
}

func TestHostLimiterContextCancelled(t *testing.T) {
	t.Parallel()

	// One event per 100s with burst 1: the first Wait consumes the token, the
	// second would block long past the cancelled context.
	h := NewHostLimiter(0.01, 1)
	ctx := context.Background()
	if err := h.Wait(ctx, "github.com"); err != nil {
		t.Fatalf("first Wait() = %v, want nil", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := h.Wait(cancelled, "github.com"); err == nil {
		t.Fatal("Wait(cancelled) = nil, want error")
	}
}

func TestRetrySucceedsFirstTry(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Retry(context.Background(), BackoffConfig{MaxAttempts: 3, Base: time.Millisecond}, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("Retry() = %v after %d calls, want nil after 1", err, calls)
	}
}

func TestRetrySucceedsAfterFailures(t *testing.T) {
	t.Parallel()

	calls := 0
	errBoom := errors.New("boom")
	err := Retry(context.Background(), BackoffConfig{MaxAttempts: 5, Base: time.Millisecond}, func() error {
		calls++
		if calls < 3 {
			return errBoom
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("Retry() = %v after %d calls, want nil after 3", err, calls)
	}
}

func TestRetryExhausts(t *testing.T) {
	t.Parallel()

	calls := 0
	errBoom := errors.New("boom")
	err := Retry(context.Background(), BackoffConfig{MaxAttempts: 3, Base: time.Millisecond}, func() error {
		calls++
		return errBoom
	})
	if !errors.Is(err, errBoom) || calls != 3 {
		t.Fatalf("Retry() = %v after %d calls, want errBoom after 3", err, calls)
	}
}

func TestRetryPermanentStops(t *testing.T) {
	t.Parallel()

	calls := 0
	errFatal := errors.New("fatal")
	err := Retry(context.Background(), BackoffConfig{MaxAttempts: 5, Base: time.Millisecond}, func() error {
		calls++
		return Permanent(errFatal)
	})
	if !errors.Is(err, errFatal) || calls != 1 {
		t.Fatalf("Retry() = %v after %d calls, want errFatal after 1 (permanent)", err, calls)
	}
}

func TestRetryContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := Retry(ctx, BackoffConfig{MaxAttempts: 5, Base: time.Hour}, func() error {
		calls++
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("Retry(cancelled ctx) = nil, want error")
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1 before backoff hit the cancelled context", calls)
	}
}

func TestRetryInvalidAttempts(t *testing.T) {
	t.Parallel()

	if err := Retry(context.Background(), BackoffConfig{MaxAttempts: 0}, func() error { return nil }); !errors.Is(err, errNoAttempts) {
		t.Fatalf("Retry(MaxAttempts=0) = %v, want errNoAttempts", err)
	}
}
