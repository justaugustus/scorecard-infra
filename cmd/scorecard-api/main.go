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

// Command scorecard-api is a cloud-agnostic, self-hosted OpenSSF Scorecard
// results API server. It serves pre-computed results from any object store and
// generates them live on demand — a read-through cache over an in-process scan
// engine ("hybrid"). It speaks the ossf/scorecard-webapp GET contract so it is a
// drop-in --base-url target for uwu-tools/scorecard-mcp.
//
// The wiring (config -> store + scanner -> orchestrator -> HTTP server) is
// implemented across the internal packages and assembled here. See
// openspec/changes/2026-08-05-add-scorecard-api-server for the design and the
// task plan.
package main

import (
	"errors"
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// errNotImplemented is a placeholder until the server wiring lands (groups 6/7).
var errNotImplemented = errors.New("not implemented yet; see the OpenSpec task plan")

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "scorecard-api:", err)
		os.Exit(1)
	}
}

// run is the real entry point, kept separate from main so it is testable.
//
// TODO(group 6/7): load configuration from the environment, open the result
// store, construct the scanner and orchestrator, and serve the HTTP contract
// (/projects, /badge, /capabilities, /health, /readyz) with graceful shutdown.
func run(_ []string) error {
	return fmt.Errorf("%w (version %s)", errNotImplemented, version)
}
