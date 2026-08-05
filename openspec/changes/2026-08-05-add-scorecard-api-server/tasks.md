# Tasks: Scorecard API Server (cloud-agnostic, hybrid cache + live)

## 0. Pre-work / decisions

- [x] 0.1 Verify PR #4665's true merge status in `ossf/scorecard`; decide whether to reuse/extend `scorecard serve`'s
      handler wiring or implement fresh and back-port (design D11) — MERGED 2025-09-10, reverted to stdlib `net/http`;
      decision: implement fresh `/projects` handlers on `net/http`, graft into `serve` later (design D11)
- [x] 0.2 Decide: import `scorecard-webapp` generated models directly vs. vendor `openapi.yaml` and regenerate (design)
      — neither; define a lean `internal/model` JSON2 mirror, format live via `pkg/scorecard.AsJSON2()` (design D13)
- [x] 0.3 Confirm the blob key + JSON2 body contract against a live `scorecard-webapp` object (design D4) — confirmed;
      `metadata` is `omitempty`, `host` includes the TLD (design D4)

## 1. Project scaffolding

- [x] 1.1 Initialize the Go module, matching the Scorecard repo's Go toolchain version (go 1.25.6)
- [x] 1.2 Add dependencies: `github.com/ossf/scorecard/v5` (v5.5.0), `gocloud.dev` (v0.46.0)
- [x] 1.3 Lay out packages: `cmd/scorecard-api`, `internal/store`, `internal/orchestrator`, `internal/scan`,
      `internal/tokens`, `internal/model` (JSON2 + provenance)
- [x] 1.4 Add `LICENSE` (Apache-2.0), `.golangci.yml` aligned with Scorecard, and meta files (PR template, CODEOWNERS,
      CONTRIBUTING, SECURITY, dependabot, CI/lint/scorecard/zizmor workflows, AGENTS.md) — sourced from the sibling
      uwu-tools repos (peribolos, org .github) since scorecard-mcp is not accessible in this environment

## 2. Core types

- [x] 2.1 Define the result model mirroring Scorecard JSON2 (`date`, `repo{name,commit}`, `scorecard{version,commit}`,
      `score`, `checks[]`, `metadata`) plus provenance (`source`, resolved commit SHA, date, version) and completeness
- [x] 2.2 Define `platform/owner/repo` parsing (default `github.com`; accept `gitlab.com`; enforce a 40-hex commit)

## 3. result-store (blob)

- [ ] 3.1 Implement `Store` over `gocloud.dev/blob`; blank-import `s3blob`, `azureblob`, `gcsblob`, `fileblob`, `memblob`
- [ ] 3.2 Open the bucket from a URL env var (no hardcoded bucket); fail fast when unset/invalid
- [ ] 3.3 Implement the key contract: `{host}/{org}/{repo}/results.json` and `{host}/{org}/{repo}/{commit}/results.json`;
      write the latest pointer on scan write-back
- [ ] 3.4 Get/Put canonical JSON2 bodies; return a not-found sentinel on miss
- [ ] 3.5 Unit tests over `memblob`; integration test over `fileblob` and MinIO (S3-compatible)

## 4. live-scan (engine + tokens)

- [ ] 4.1 Implement `Scanner` wrapping `pkg/scorecard.Run`; create GitHub/GitLab, OSS-Fuzz, CII, and vulnerabilities
      clients once and reuse them across scans
- [ ] 4.2 Format results to JSON2; on success write the result back to the store (populate the cache)
- [ ] 4.3 Handle skip (unreachable/blocked → skipped, not fatal) and fatal errors distinctly
- [ ] 4.4 Implement `internal/tokens`: SCM token pool (GitHub App / PAT), per-host rate limiter, backoff/retry
- [ ] 4.5 Bound live scans with an in-process worker pool (concurrency from config)
- [ ] 4.6 Unit tests with a fake scanner; declare live coverage in capabilities (all checks; any accessible repo)

## 5. result-cache (orchestrator)

- [ ] 5.1 Implement `GetOrProduce(ref, commit)`: store lookup → freshness check → serve, else scan+persist+serve
- [ ] 5.2 Freshness policy: commit-pinned = immutable; `latest` = TTL (from config)
- [ ] 5.3 Single-flight de-duplication so concurrent identical requests trigger exactly one scan
- [ ] 5.4 Sync-with-timeout vs. async: return `200` within the timeout, else `202` + `Retry-After`
- [ ] 5.5 Attach provenance (source, commit SHA, date, version) and completeness to every result
- [ ] 5.6 Unit tests: hit-fresh, miss→scan, stale→refresh, concurrent coalescing, timeout→202

## 6. api-server (HTTP)

- [ ] 6.1 Implement `GET /projects/{host}/{org}/{repo}` (+ optional `?commit=`) returning JSON2 via the orchestrator
- [ ] 6.2 Implement `GET /projects/{host}/{org}/{repo}/badge` (SVG)
- [ ] 6.3 Implement `GET /capabilities` advertising source/mode, check set, opt-in=false, freshness policy, caveats
- [ ] 6.4 Implement `GET /health` and `GET /readyz`
- [ ] 6.5 Error handling: unknown/never-scanned + scan failure → clear status codes and messages; responsible framing
- [ ] 6.6 Graceful shutdown on SIGINT/SIGTERM; listen port + timeouts from config
- [ ] 6.7 Handler tests for each route (hit, miss→live, commit-pinned, badge, capabilities, health)

## 7. Configuration

- [ ] 7.1 Load all config from env: bucket URL, `latest` TTL, request/scan timeouts, enabled checks, worker
      concurrency, listen port, SCM credentials; document each with defaults
- [ ] 7.2 Startup validation: fail fast with actionable errors on missing/invalid required config

## 8. Acceptance: scorecard-mcp as the client

- [ ] 8.1 Integration test: run the server on `fileblob`; `scorecard-mcp --base-url http://localhost:PORT get_repo_score`
      returns a correct result on a cache HIT
- [ ] 8.2 Integration test: cache MISS triggers a live `scorecard.Run()` that populates the bucket and serves it;
      repeat over MinIO
- [ ] 8.3 Verify `scorecard-mcp` receives correct provenance/caveats (manually, pending the MCP `/capabilities` reader)

## 9. Testing and verification

- [ ] 9.1 Map each spec scenario (api-server, result-store, result-cache, live-scan) to a test case
- [ ] 9.2 Run `golangci-lint` (0 issues) and `go test ./...` clean
- [ ] 9.3 Manual smoke test: `curl` each route against a local `fileblob` bucket and MinIO

## 10. Documentation

- [ ] 10.1 README: what it is, the contract, cloud-agnostic backends (S3/MinIO/Azure/GCS/file), env config, and the
      `scorecard-mcp --base-url` example; state the hybrid (cached+live) behavior and caveats
- [ ] 10.2 Document the upstream graft map and the `scorecard serve` reconciliation status in `docs/`

## 11. Change closeout

- [ ] 11.1 `openspec validate add-scorecard-api-server --strict` passes against the implemented behavior
- [ ] 11.2 Update `AGENTS.md`/README if any convention changed during implementation
- [ ] 11.3 Archive the OpenSpec change once implemented and merged
