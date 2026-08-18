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

package model

// Source indicates how a served result was obtained.
type Source string

const (
	// SourceCached means the result was served from the store.
	SourceCached Source = "cached"
	// SourceLive means the result was produced by an on-demand scan.
	SourceLive Source = "live"
	// SourceUpstream means the result was fetched from an upstream Scorecard API
	// (the result fallback), not produced locally.
	SourceUpstream Source = "upstream"
)

// Provenance describes where a served result came from and how complete it is.
// The server declares it out of band (HTTP response headers and /capabilities),
// never inside the canonical JSON2 body, so webapp-compatible clients parse the
// body unchanged (design D4/D12).
//
// Scorecard results are heuristic signals, not a verdict: provenance lets a
// client report source, freshness, and completeness rather than assume them.
type Provenance struct {
	// Source is whether the result was cached or freshly scanned.
	Source Source
	// Commit is the resolved commit SHA the result was computed at.
	Commit string
	// Date is the result's JSON2 generation date.
	Date string
	// ScorecardVersion is the engine version that produced the result.
	ScorecardVersion string
	// Complete reports whether every check produced a conclusive (non-negative)
	// score. This one source-agnostic rule applies to live, cached, and upstream
	// results alike (see scan.Complete).
	Complete bool
}

// ProvenanceFrom derives provenance from a parsed result and the source it was
// obtained from.
func ProvenanceFrom(r *Result, src Source, complete bool) Provenance {
	return Provenance{
		Source:           src,
		Commit:           r.Repo.Commit,
		Date:             r.Date,
		ScorecardVersion: r.Scorecard.Version,
		Complete:         complete,
	}
}
