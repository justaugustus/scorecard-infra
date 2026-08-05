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

package model

import (
	"encoding/json"
	"fmt"
)

// Result mirrors the canonical Scorecard JSON2 result body, matching the field
// layout and JSON tags of pkg/scorecard.JSONScorecardResultV2.
//
// It is used to introspect a result (score, resolved commit, generation date,
// version); the server serves the original JSON2 bytes rather than re-marshaling
// this struct, so wire compatibility never depends on it.
//
// Metadata uses omitempty because pkg/scorecard emits "metadata": null while a
// Scorecard-webapp response omits it entirely; a []string decodes both forms.
//
//nolint:govet // keep canonical Scorecard JSON2 field order over pointer packing
type Result struct {
	Date      string    `json:"date"`
	Repo      Repo      `json:"repo"`
	Scorecard Scorecard `json:"scorecard"`
	Score     float64   `json:"score"`
	Checks    []Check   `json:"checks"`
	Metadata  []string  `json:"metadata,omitempty"`
}

// Repo is the analyzed repository and the commit the result was computed at.
type Repo struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// Scorecard is the version of the Scorecard engine that produced the result.
type Scorecard struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// Check is a single Scorecard check result. Details is nullable in the wire
// format; Annotations is present only when annotations were requested.
//
//nolint:govet // keep canonical Scorecard JSON2 field order over pointer packing
type Check struct {
	Details       []string      `json:"details"`
	Score         int           `json:"score"`
	Reason        string        `json:"reason"`
	Name          string        `json:"name"`
	Documentation Documentation `json:"documentation"`
	Annotations   []string      `json:"annotations,omitempty"`
}

// Documentation links a check to its human-readable explanation.
type Documentation struct {
	URL   string `json:"url"`
	Short string `json:"short"`
}

// Parse decodes canonical Scorecard JSON2 bytes into a Result for introspection.
func Parse(data []byte) (*Result, error) {
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("model: decoding JSON2 result: %w", err)
	}
	return &r, nil
}
