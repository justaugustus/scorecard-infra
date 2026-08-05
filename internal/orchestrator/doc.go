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

// Package orchestrator is the read-through cache and the central serve-vs-scan
// seam (design D2). Every request flows through GetOrProduce, which looks up the
// store, checks freshness, and either serves the cached result or triggers a
// live scan, persists it, and returns it.
//
// It owns the freshness policy (commit-pinned = immutable; latest = TTL, D5),
// single-flight de-duplication so concurrent identical requests trigger exactly
// one scan (D6), the synchronous-with-timeout vs. asynchronous 202 decision
// (D5), and provenance stamping on every result (D12). It depends only on a
// Store and a Scanner interface, so both backends are swappable and testable.
//
// TODO(group 5): implement GetOrProduce and the freshness/single-flight/response
// policies here.
package orchestrator
