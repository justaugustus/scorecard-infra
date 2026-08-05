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

package httpapi

import (
	"context"
	"testing"
	"time"
)

func TestListenAndServeGracefulShutdown(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{}, DefaultCapabilities(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		// Port 0 binds an ephemeral port; we only exercise the shutdown path.
		done <- srv.ListenAndServe(ctx, ServeConfig{Addr: "127.0.0.1:0", ShutdownTimeout: time.Second})
	}()

	time.Sleep(20 * time.Millisecond) // let the listener come up
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListenAndServe = %v, want nil on graceful shutdown", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}
