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

import "time"

// Capabilities advertises this server's mode, coverage, and caveats so clients
// (e.g. scorecard-mcp) report provenance from the server instead of assuming
// public-cache behavior (design D7). Field order favors pointer packing; JSON
// tags fix the wire shape.
type Capabilities struct {
	// Mode is the sourcing model, "cached+live".
	Mode string `json:"mode"`
	// Checks describes the check coverage, "all".
	Checks string `json:"checks"`
	// Caveats are human-readable provenance notes clients should surface.
	Caveats []string `json:"caveats"`
	// LatestTTLSeconds is how long a latest result stays fresh.
	LatestTTLSeconds int `json:"latest_ttl_seconds"`
	// RequiresOptIn reports whether coverage needs publish_results (false here).
	RequiresOptIn bool `json:"requires_opt_in"`
}

// DefaultCapabilities returns the capabilities of this hybrid server: it scans
// on demand, runs all checks against any accessible repository, requires no
// opt-in, and caches latest results with the given TTL.
func DefaultCapabilities(latestTTL time.Duration) Capabilities {
	return Capabilities{
		Mode:             "cached+live",
		Checks:           "all",
		LatestTTLSeconds: int(latestTTL / time.Second),
		RequiresOptIn:    false,
		Caveats: []string{
			"Scorecard results are heuristic signals, not a verdict; a repository is never labeled secure or insecure.",
			"A score of -1 is inconclusive, not a failing score.",
			"Results are generated on demand for any repository the configured token can access; " +
				"no publish_results opt-in is required.",
			"Latest results are cached with a TTL and refreshed on expiry; pin a commit for an immutable result.",
		},
	}
}

// WithFallback returns a copy of c advertising the upstream result fallback: it
// updates the mode and appends caveats describing the upstream's limits, so a
// client reports provenance accurately when a result is served from upstream
// (design F2/D7).
func (c Capabilities) WithFallback() Capabilities {
	c.Mode = "cached+upstream+live"
	caveats := make([]string, len(c.Caveats), len(c.Caveats)+2)
	copy(caveats, c.Caveats)
	caveats = append(caveats,
		"When a fresh local result is unavailable, a result may be served from an upstream Scorecard API; "+
			"such results may be up to a week old, may omit some checks, and cover only repositories that "+
			"opted in upstream.",
		`An upstream-sourced result is reported with source "upstream" and its own generation date.`,
	)
	c.Caveats = caveats
	return c
}
