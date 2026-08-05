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

- [x] 3.1 Implement `Store` over `gocloud.dev/blob`; blank-import `s3blob`, `azureblob`, `gcsblob`, `fileblob`, `memblob`
- [x] 3.2 Open the bucket from a URL (no hardcoded bucket); fail fast when unset/invalid — env-var binding lands in
      config (task 7.1); `Open` already errors on an empty/invalid URL
- [x] 3.3 Implement the key contract: `{host}/{org}/{repo}/results.json` and `{host}/{org}/{repo}/{commit}/results.json`;
      write the latest pointer on scan write-back (`PutLatestAndCommit`)
- [x] 3.4 Get/Put canonical JSON2 bodies; return a not-found sentinel on miss (`ErrNotFound`)
- [x] 3.5 Unit tests over `memblob`; integration test over `fileblob` (runs) and MinIO/S3 (gated on
      `SCORECARD_TEST_S3_URL`, skipped when no endpoint is available)

## 4. live-scan (engine + tokens)

- [x] 4.1 Implement `Scanner` wrapping `pkg/scorecard.Run`; reuse OSS-Fuzz/CII/vulnerabilities clients across scans;
      create the stateful SCM repo client per scan (unsafe to share concurrently) — `EngineScanner`
- [x] 4.2 Format results to JSON2 (`AsJSON2`); write-back to the store is performed by the orchestrator (group 5),
      which owns the latest-vs-commit key policy (design D2) — the scanner returns the JSON2 + resolved commit
- [x] 4.3 Handle skip (unreachable/blocked → `ErrSkipped`, not retried) and fatal errors distinctly
- [x] 4.4 Implement `internal/tokens`: PAT pool (+ `Joined` for Scorecard's GITHUB_AUTH_TOKEN rotation), per-host
      rate limiter, backoff/retry with a `Permanent` sentinel
- [x] 4.5 Bound live scans with an in-process worker pool (`Bounded`; concurrency from config)
- [x] 4.6 Unit tests with a fake scanner (`FakeScanner`); live coverage (all checks; any accessible repo; no opt-in)
      documented for `/capabilities` (group 6). EngineScanner is compile-verified against v5.5.0 (no network here)

## 5. result-cache (orchestrator)

- [x] 5.1 Implement `GetOrProduce(ref, commit)`: store lookup → freshness check → serve, else scan+persist+serve
- [x] 5.2 Freshness policy: commit-pinned = immutable; `latest` = TTL (from config)
- [x] 5.3 Single-flight de-duplication so concurrent identical requests trigger exactly one scan (`singleflight.DoChan`)
- [x] 5.4 Sync-with-timeout vs. async: return ready (`200`) within the timeout, else not-ready (`202`) + `RetryAfter`
- [x] 5.5 Attach provenance (source, commit SHA, date, version) and completeness to every result
- [x] 5.6 Unit tests: hit-fresh, miss→scan, stale→refresh, commit immutability, concurrent coalescing, timeout→202,
      skip propagation, and -1 preservation

## 6. api-server (HTTP)

- [x] 6.1 Implement `GET /projects/{host}/{org}/{repo}` (+ optional `?commit=`) returning JSON2 via the orchestrator
- [x] 6.2 Implement `GET /projects/{host}/{org}/{repo}/badge` (SVG)
- [x] 6.3 Implement `GET /capabilities` advertising source/mode, check set, opt-in=false, freshness policy, caveats
- [x] 6.4 Implement `GET /health` and `GET /readyz` (readiness probe injectable)
- [x] 6.5 Error handling: malformed→400, skipped/unreachable→404, scan failure→502, not-ready→202; responsible framing
- [x] 6.6 Graceful shutdown via `ListenAndServe` (context-driven; SIGINT/SIGTERM→ctx wiring in main, group 7); listen
      port + timeouts from config
- [x] 6.7 Handler tests for each route (ready, malformed, bad commit, skip→404, fail→502, 202, badge, capabilities,
      health, readyz, method-not-allowed, graceful shutdown)

## 7. Configuration

- [x] 7.1 Load all config from env: bucket URL, `latest` TTL, request/scan timeouts, enabled checks, worker
      concurrency, listen port, SCM credentials; document each with defaults (`internal/config`)
- [x] 7.2 Startup validation: fail fast with actionable errors on missing/invalid required config (verified via
      `go run` with no bucket URL)

## 8. Acceptance: scorecard-mcp as the client

> DEFERRED — requires the `scorecard-mcp` client (not accessible in this environment)
> plus network + an SCM token for a live scan. To be completed once those are available.

- [ ] 8.1 Integration test: run the server on `fileblob`; `scorecard-mcp --base-url http://localhost:PORT get_repo_score`
      returns a correct result on a cache HIT
- [ ] 8.2 Integration test: cache MISS triggers a live `scorecard.Run()` that populates the bucket and serves it;
      repeat over MinIO
- [ ] 8.3 Verify `scorecard-mcp` receives correct provenance/caveats (manually, pending the MCP `/capabilities` reader)

## 9. Testing and verification

- [x] 9.1 Map each spec scenario (api-server, result-store, result-cache, live-scan) to a test case — covered by unit
      tests per package; the live-engine and scorecard-mcp-compat scenarios are exercised via the fake seam and
      deferred to group 8 for a real run
- [x] 9.2 Run `golangci-lint` (0 issues) and `go test ./...` clean — plus `actionlint` and `zizmor` on workflows
- [ ] 9.3 Smoke test each route against a local `fileblob` bucket and MinIO — PARTIAL: done offline via an end-to-end
      integration test (`internal/httpapi/integration_test.go`: real HTTP -> orchestrator -> fileblob, covering HIT,
      MISS->scan->persist->re-HIT, badge, capabilities, health). Remaining: a live MISS against a real scan and the
      MinIO backend (needs network/token/Docker) — folded into group 8

## 10. Documentation

- [x] 10.1 README: what it is, the contract, cloud-agnostic backends (S3/MinIO/Azure/GCS/file), env config, and the
      `scorecard-mcp --base-url` example; state the hybrid (cached+live) behavior and caveats
- [x] 10.2 Document the upstream graft map and the `scorecard serve` reconciliation status in `docs/` — added
      `docs/upstream-graft.md` (per-component graft map + D11/#4665 reconciliation); README/AGENTS link to it

## 11. Change closeout

- [x] 11.1 `openspec validate 2026-08-05-add-scorecard-api-server --strict` passes against the implemented behavior
- [x] 11.2 Update `AGENTS.md`/README if any convention changed during implementation — AGENTS.md updated with the
      lint conventions that emerged and a status note (README is group 10, not yet written)
- [ ] 11.3 Archive the OpenSpec change once implemented and merged — DEFERRED until group 8 acceptance is done and the
      change is merged to `main`
