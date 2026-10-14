// Copyright 2026 OpenSSF Scorecard Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"regexp"

	"actionlint.kjanat.dev"
	"go.yaml.in/yaml/v4"
)

// reviewedKey is a workflow key that, set to value, can only take capability
// away from a job. A parser that predates the key reports it as unexpected;
// that diagnostic is let through for this exact value and nothing else.
//
// Additions go through normal code review, and only for values that remove
// capability. Values that keep or grant access (cache-mode's read, write, and
// write-only) stay off the list.
type reviewedKey struct {
	key     string
	section string
	value   string
}

var reviewedKeys = []reviewedKey{
	// https://github.com/ossf/scorecard/issues/5322
	{key: "cache-mode", section: "workflow", value: "none"},
	{key: "cache-mode", section: "job", value: "none"},
}

var unexpectedKeyRE = regexp.MustCompile(`^unexpected key "([^"]+)" for "([^"]+)" section`)

// unreviewedDiagnostics returns the parser diagnostics that still reject the
// workflow. Everything is kept unless it is an unexpected-key diagnostic for
// a reviewed key, and the YAML at the diagnostic's position sets that key to
// its reviewed value. Malformed YAML, unknown keys, and any other value all
// still fail verification.
func unreviewedDiagnostics(content string, diags []*actionlint.Error) []*actionlint.Error {
	if len(diags) == 0 {
		return nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return diags
	}
	var kept []*actionlint.Error
	for _, d := range diags {
		if !isReviewed(&root, d) {
			kept = append(kept, d)
		}
	}
	return kept
}

func isReviewed(root *yaml.Node, d *actionlint.Error) bool {
	if d.Kind != "syntax-check" {
		return false
	}
	m := unexpectedKeyRE.FindStringSubmatch(d.Message)
	if m == nil {
		return false
	}
	value := valueAt(root, d.Line, d.Column, m[1])
	if value == nil || value.Kind != yaml.ScalarNode {
		return false
	}
	for _, r := range reviewedKeys {
		if r.key == m[1] && r.section == m[2] && r.value == value.Value {
			return true
		}
	}
	return false
}

// valueAt finds the mapping entry whose key node is key at line and column,
// and returns its value node.
func valueAt(n *yaml.Node, line, column int, key string) *yaml.Node {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Line == line && k.Column == column && k.Value == key {
				return n.Content[i+1]
			}
		}
	}
	for _, c := range n.Content {
		if v := valueAt(c, line, column, key); v != nil {
			return v
		}
	}
	return nil
}
