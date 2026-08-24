/*
Copyright 2026 OpenSSF Scorecard Authors.

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

// Package model defines a lean in-repo mirror of the OpenSSF Scorecard JSON2
// wire format ([Result]), the repository reference the server routes on
// ([RepoRef]), and the provenance metadata ([Provenance]) that the server
// declares for every response.
//
// Per design decision D13, this package deliberately does NOT import the
// go-swagger-generated models from ossf/scorecard-webapp (which drag in the full
// go-openapi runtime).
//
// Note that half of D13's premise has expired: those generated models are no
// longer only upstream. They were imported into this repository as
// api/app/generated/models, and the go-openapi runtime is now a direct
// dependency of this module regardless of what this package does. The remaining
// argument is narrower but still holds — this package exists to unmarshal a
// handful of fields, and depending on the generated models to do it would couple
// the cache path to the frozen tree that presubmits.yml forbids importing.
// Revisit when the two serving-tier implementations converge (design W10).
//
// Live results are formatted via pkg/scorecard.AsJSON2()
// and cached bytes are passed through unchanged; [Result] exists only to
// unmarshal the fields the server must introspect (score for the badge; repo
// commit, date, and version for freshness and provenance). It mirrors the
// canonical shape of pkg/scorecard.JSONScorecardResultV2 exactly.
//
// Provenance is delivered out of band (HTTP response headers and /capabilities),
// never embedded in the canonical JSON2 body, so webapp-compatible clients such
// as scorecard-mcp parse the body unchanged (design D4/D12).
package model
