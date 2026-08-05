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
	"fmt"
	"net/http"

	"github.com/uwu-tools/scorecard-api/internal/model"
)

// Badge colors, following the shields.io convention.
const (
	colorGreen  = "#4c1"
	colorYellow = "#dfb317"
	colorRed    = "#e05d44"
	colorGray   = "#9f9f9f"
	badgeLabel  = "scorecard"
)

// svgTemplate is a minimal, self-contained badge. A richer renderer can be
// grafted from scorecard-webapp later (design open question); this avoids a
// heavy dependency for v0.
const svgTemplate = `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
<title>%s: %s</title>
<rect rx="3" width="%d" height="20" fill="#555"/>
<rect rx="3" x="%d" width="%d" height="20" fill="%s"/>
<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
<text x="%d" y="14">%s</text>
<text x="%d" y="14">%s</text>
</g>
</svg>`

// handleBadge serves GET /projects/{host}/{org}/{repo}/badge as an SVG badge.
func (s *Server) handleBadge(w http.ResponseWriter, r *http.Request) {
	ref, commit, ok := s.refFromPath(w, r)
	if !ok {
		return
	}

	out, err := s.producer.GetOrProduce(r.Context(), ref, commit)
	if err != nil {
		s.writeProduceError(w, ref, err)
		return
	}
	if !out.Ready {
		s.writeNotReady(w, out)
		return
	}

	parsed, perr := model.Parse(out.Body)
	if perr != nil {
		s.writeError(w, http.StatusBadGateway, "invalid result", "the stored result could not be parsed")
		return
	}

	s.setProvenance(w, out.Provenance)
	w.Header().Set("Content-Type", "image/svg+xml;charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write([]byte(renderBadge(parsed.Score))); werr != nil {
		s.logger.Warn("writing badge", "error", werr)
	}
}

// renderBadge produces an SVG badge for an aggregate score. A negative score is
// Scorecard's inconclusive sentinel and is rendered as such, not as a failure.
func renderBadge(score float64) string {
	var value, color string
	switch {
	case score < 0:
		value, color = "inconclusive", colorGray
	case score >= 8:
		value, color = fmt.Sprintf("%.1f", score), colorGreen
	case score >= 5:
		value, color = fmt.Sprintf("%.1f", score), colorYellow
	default:
		value, color = fmt.Sprintf("%.1f", score), colorRed
	}

	const charW, pad = 7, 10
	labelW := charW*len(badgeLabel) + pad
	valueW := charW*len(value) + pad
	total := labelW + valueW

	return fmt.Sprintf(svgTemplate,
		total, badgeLabel, value, // svg width + aria-label
		badgeLabel, value, // title
		labelW,                // label background rect width
		labelW, valueW, color, // value background rect x/width/color
		labelW/2, badgeLabel, // label text x + content
		labelW+valueW/2, value, // value text x + content
	)
}
