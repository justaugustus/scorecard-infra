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
	"runtime/debug"
	"sync"

	"github.com/go-openapi/runtime/middleware"

	"github.com/ossf/scorecard-infra/api/app/generated/models"
	"github.com/ossf/scorecard-infra/api/app/generated/restapi/operations/version"
)

// Set at link time by api/Makefile from the GIT_VERSION and GIT_HASH build
// args publish-images.yml passes. The Docker build has no .git, so the Go
// toolchain's own VCS stamping can't supply them.
var (
	gitVersion string
	gitCommit  string
)

const (
	unknown = "unknown"

	// The workflow parser is the dependency whose version decides which
	// uploads verify; reporting it is what makes a rejection traceable.
	parserModule = "actionlint.kjanat.dev"

	// The response only changes on deploy, so the CDN absorbs repeat traffic.
	// After a deploy, purge the key or wait out the TTL to see the new build.
	versionFastlyTTL    = "max-age=300"
	versionBrowserTTL   = "max-age=60"
	versionSurrogateKey = "scorecard-version"
)

// Computed once: the answer can't change while the process runs, and a
// request should cost no more than writing it out.
var buildVersion = sync.OnceValue(func() *models.VersionInfo {
	info, _ := debug.ReadBuildInfo()
	return newVersionInfo(gitVersion, gitCommit, info)
})

func newVersionInfo(ver, commit string, info *debug.BuildInfo) *models.VersionInfo {
	return &models.VersionInfo{
		Version: orUnknown(ver),
		Commit:  orUnknown(commit),
		Parser:  moduleVersion(info, parserModule),
	}
}

// moduleVersion reports path's module and version as linked, following a
// replace directive so a fork or local override is reported as what it is.
func moduleVersion(info *debug.BuildInfo, path string) string {
	if info == nil {
		return unknown
	}
	for _, dep := range info.Deps {
		if dep.Path != path {
			continue
		}
		if dep.Replace != nil {
			dep = dep.Replace
		}
		return orUnknown(dep.Path + " " + dep.Version)
	}
	return unknown
}

func orUnknown(s string) string {
	if s == "" {
		return unknown
	}
	return s
}

func GetVersionHandler(_ version.GetVersionParams) middleware.Responder {
	return version.NewGetVersionOK().WithPayload(buildVersion()).
		WithSurrogateControl(versionFastlyTTL).
		WithCacheControl(versionBrowserTTL).
		WithSurrogateKey(versionSurrogateKey)
}
