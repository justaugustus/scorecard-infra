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

package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ossf/scorecard/v5/checker"
	"github.com/ossf/scorecard/v5/clients"
	"github.com/ossf/scorecard/v5/clients/githubrepo"
	"github.com/ossf/scorecard/v5/clients/gitlabrepo"
	"github.com/ossf/scorecard/v5/clients/ossfuzz"
	docs "github.com/ossf/scorecard/v5/docs/checks"
	sce "github.com/ossf/scorecard/v5/errors"
	sclog "github.com/ossf/scorecard/v5/log"
	"github.com/ossf/scorecard/v5/pkg/scorecard"
	"github.com/ossf/scorecard/v5/policy"

	"github.com/uwu-tools/scorecard-api/internal/model"
	"github.com/uwu-tools/scorecard-api/internal/tokens"
)

// ipAllowListMarker is the substring Scorecard surfaces when a repository is
// behind an org IP allow list; the cron worker treats it as a skip, and so do we.
const ipAllowListMarker = "organization has an IP allow list"

// EngineScanner is the live Scanner backed by pkg/scorecard.Run. The OSS-Fuzz,
// CII, and vulnerabilities clients are created once and reused across scans; the
// SCM repo client is created per scan because it is stateful per repository.
//
//nolint:govet // field order documents client reuse; this is a lifetime singleton
type EngineScanner struct {
	logger    *sclog.Logger
	ossFuzz   clients.RepoClient
	vuln      clients.VulnerabilitiesClient
	cii       clients.CIIBestPracticesClient
	limiter   *tokens.HostLimiter
	checkDocs docs.Doc
	checks    []string
	backoff   tokens.BackoffConfig
	logLevel  sclog.Level
}

// EngineConfig configures an EngineScanner.
type EngineConfig struct {
	// Limiter paces scans per SCM host. Nil means unlimited.
	Limiter *tokens.HostLimiter
	// LogLevel is the Scorecard engine log level (e.g. "info").
	LogLevel string
	// Checks optionally restricts the checks to run; empty means all default checks.
	Checks []string
	// Backoff governs retries of a scan. A zero value uses tokens.DefaultBackoff.
	Backoff tokens.BackoffConfig
}

// Ensure EngineScanner satisfies the Scanner interface.
var _ Scanner = (*EngineScanner)(nil)

// NewEngineScanner constructs a live scanner, eagerly creating and reusing the
// auxiliary clients. It reads the check documentation once. The OSS-Fuzz client
// loads its status data eagerly, so this performs network I/O and should be
// called at server startup.
func NewEngineScanner(cfg EngineConfig) (*EngineScanner, error) {
	level := sclog.ParseLevel(cfg.LogLevel)
	logger := sclog.NewLogger(level)

	checkDocs, err := docs.Read()
	if err != nil {
		return nil, fmt.Errorf("scan: reading check docs: %w", err)
	}

	ossFuzz, err := ossfuzz.CreateOSSFuzzClientEager(ossfuzz.StatusURL)
	if err != nil {
		return nil, fmt.Errorf("scan: creating OSS-Fuzz client: %w", err)
	}

	limiter := cfg.Limiter
	if limiter == nil {
		limiter = tokens.NewHostLimiter(0, 0)
	}
	backoff := cfg.Backoff
	if backoff.MaxAttempts < 1 {
		backoff = tokens.DefaultBackoff()
	}

	return &EngineScanner{
		logger:    logger,
		ossFuzz:   ossFuzz,
		vuln:      clients.DefaultVulnerabilitiesClient(),
		cii:       clients.DefaultCIIBestPracticesClient(),
		limiter:   limiter,
		checkDocs: checkDocs,
		checks:    cfg.Checks,
		backoff:   backoff,
		logLevel:  level,
	}, nil
}

// Close releases the reused clients.
func (s *EngineScanner) Close() error {
	if err := s.ossFuzz.Close(); err != nil {
		return fmt.Errorf("scan: closing OSS-Fuzz client: %w", err)
	}
	return nil
}

// Scan runs Scorecard for ref at commit (empty = HEAD) and returns the JSON2
// result. A skipped repository yields ErrSkipped; other failures return a
// wrapped error.
func (s *EngineScanner) Scan(ctx context.Context, ref model.RepoRef, commit string) (*Result, error) {
	if err := s.limiter.Wait(ctx, ref.Host); err != nil {
		return nil, err
	}

	repo, repoClient, err := s.repoClient(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := repoClient.Close(); cerr != nil {
			s.logger.Error(cerr, "closing repo client")
		}
	}()

	commitSHA := clients.HeadSHA
	var required []checker.RequestType
	if commit != "" {
		commitSHA = commit
		required = append(required, checker.CommitBased)
	}

	enabled, err := policy.GetEnabled(nil, s.checks, required, repo.Type())
	if err != nil {
		return nil, fmt.Errorf("scan: policy.GetEnabled: %w", err)
	}
	checks := make([]string, 0, len(enabled))
	for name := range enabled {
		checks = append(checks, name)
	}

	res, err := s.run(ctx, repo, repoClient, commitSHA, checks)
	if errors.Is(err, ErrSkipped) {
		return nil, ErrSkipped
	}
	if err != nil {
		return nil, err
	}

	// Deterministic output: sort checks by name, matching `scorecard serve`.
	sort.Slice(res.Checks, func(i, j int) bool { return res.Checks[i].Name < res.Checks[j].Name })

	var buf bytes.Buffer
	if formatErr := res.AsJSON2(&buf, s.checkDocs, &scorecard.AsJSON2ResultOption{
		LogLevel: s.logLevel,
		Details:  true,
	}); formatErr != nil {
		return nil, fmt.Errorf("scan: formatting JSON2: %w", formatErr)
	}

	parsed, err := model.Parse(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("scan: parsing produced JSON2: %w", err)
	}

	return &Result{
		JSON2:          buf.Bytes(),
		ResolvedCommit: parsed.Repo.Commit,
		Complete:       Complete(parsed.Checks),
	}, nil
}

// run invokes scorecard.Run with the reused auxiliary clients under the backoff
// policy, translating a skip condition into ErrSkipped (which it never retries).
func (s *EngineScanner) run(
	ctx context.Context,
	repo clients.Repo,
	repoClient clients.RepoClient,
	commitSHA string,
	checks []string,
) (scorecard.Result, error) {
	var res scorecard.Result
	err := tokens.Retry(ctx, s.backoff, func() error {
		var runErr error
		res, runErr = scorecard.Run(ctx, repo,
			scorecard.WithLogLevel(s.logLevel),
			scorecard.WithCommitSHA(commitSHA),
			scorecard.WithChecks(checks),
			scorecard.WithRepoClient(repoClient),
			scorecard.WithOSSFuzzClient(s.ossFuzz),
			scorecard.WithOpenSSFBestPraticesClient(s.cii),
			scorecard.WithVulnerabilitiesClient(s.vuln),
		)
		if isSkip(runErr) {
			// A skip is terminal, not a transient failure: stop retrying.
			return tokens.Permanent(ErrSkipped)
		}
		if runErr != nil {
			return fmt.Errorf("scan: scorecard.Run: %w", runErr)
		}
		return nil
	})
	return res, err
}

// repoClient builds the SCM repo and a fresh repo client for ref.
func (s *EngineScanner) repoClient(ctx context.Context, ref model.RepoRef) (clients.Repo, clients.RepoClient, error) {
	if ref.Host == model.HostGitLab {
		repo, err := gitlabrepo.MakeGitlabRepo(ref.Name())
		if err != nil {
			return nil, nil, fmt.Errorf("scan: making GitLab repo %q: %w", ref.Name(), err)
		}
		client, err := gitlabrepo.CreateGitlabClient(ctx, ref.Host)
		if err != nil {
			return nil, nil, fmt.Errorf("scan: creating GitLab client: %w", err)
		}
		return repo, client, nil
	}

	repo, err := githubrepo.MakeGithubRepo(ref.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("scan: making GitHub repo %q: %w", ref.Name(), err)
	}
	return repo, githubrepo.CreateGithubRepoClient(ctx, s.logger), nil
}

// isSkip reports whether err is a non-fatal skip condition.
func isSkip(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sce.ErrRepoUnreachable) {
		return true
	}
	return strings.Contains(err.Error(), ipAllowListMarker)
}
