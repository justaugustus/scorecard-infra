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

// Package model defines a lean in-repo mirror of the OpenSSF Scorecard JSON2
// wire format, plus provenance metadata (source, resolved commit, date,
// version) that this server attaches to every response.
//
// Per design decision D13, this package deliberately does NOT import the
// go-swagger-generated models from ossf/scorecard-webapp (which drag in the full
// go-openapi runtime). Live results are formatted via pkg/scorecard.AsJSON2()
// and cached bytes are passed through unchanged; this model exists only to
// unmarshal the fields the server must introspect (score for the badge; repo
// commit and date for freshness/provenance).
//
// The JSON2 body shape (confirmed against a live api.scorecard.dev object,
// task 0.3): {date, repo{name,commit}, scorecard{version,commit}, score,
// checks[]{name,score,reason,details,documentation{short,url}}, metadata}.
// The metadata field is omitted when empty and details is nullable.
//
// TODO(group 2): define the result and provenance types here.
package model
