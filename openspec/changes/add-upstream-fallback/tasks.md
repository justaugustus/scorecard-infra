# Tasks: Upstream result fallback (api.scorecard.dev)

## 0. Pre-work / decisions

- [ ] 0.1 Confirm `api.scorecard.dev` serves the `/projects/{host}/{org}/{repo}`
      GET contract as canonical JSON2, and the status for an unknown/opted-out repo
      (expect 404 = miss) — PENDING a live check; implemented to the documented
      contract (design F8), exercised against an httptest stub
- [x] 0.2 Confirm blob metadata is available for the origin tag — verified for
      `memblob` and `fileblob` via store tests; S3 uses the same gocloud metadata

## 1. Configuration and flags

- [x] 1.1 Add `SCORECARD_FALLBACK_URL`, `SCORECARD_FALLBACK_TIMEOUT` (5s),
      `SCORECARD_FALLBACK_MAX_AGE` (7d) with validation
- [x] 1.2 Declare `fallback.enabled` (bool, default true) and `fallback.mode`
      (string, default `fetch-first`, allowed {fetch-first, safety-net}); wire them
      in `cmd/scorecard-api` and capture the flag client for the orchestrator

## 2. Fallback client (`internal/fallback`)

- [x] 2.1 Define the `Fallback` interface (in orchestrator) and `ErrFallbackMiss`
- [x] 2.2 HTTP client: `GET {url}/projects/...`, parse via `model.Parse`, 404 →
      miss, other errors → non-fatal; honor the timeout
- [x] 2.3 Enforce the max-age bound: a result older than max-age is a miss
- [x] 2.4 Unit-test hit, miss (404), upstream error, transport error, stale

## 3. Model

- [x] 3.1 Add `model.SourceUpstream`
- [x] 3.2 Affirm the uniform completeness rule (conclusive, source-agnostic)

## 4. Store origin tag

- [x] 4.1 Persist/retrieve an origin tag as blob metadata; untagged = local
- [x] 4.2 `GetWithOrigin` returns the origin alongside the body
- [x] 4.3 Unit-test the round-trip and the legacy-untagged default

## 5. Orchestrator integration

- [x] 5.1 Optional `Fallback` + flag client via options; read the flags per request
- [x] 5.2 Mode-aware ordering: fetch-first (upstream → scan) and safety-net (scan →
      upstream on terminal skip/error)
- [x] 5.3 Source-aware freshness (TTL for local, max-age for upstream); report the
      true source on store hits
- [x] 5.4 Backfill a used upstream result tagged upstream; commit-pinned bypass;
      fetch stays outside the singleflight/202 path
- [x] 5.5 Unit-test both modes with fakes: fetch-first hit/backfill, miss→scan,
      safety-net success/skip-rescue, commit bypass, kill-switch, source-aware
      freshness (fresh + stale)

## 6. HTTP surface / capabilities

- [x] 6.1 `/capabilities` reflects the fallback mode and caveats when active
- [x] 6.2 `X-Scorecard-Source: upstream` flows through the provenance path (tested)

## 7. Testing and verification

- [x] 7.1 Map each spec scenario (upstream-fallback, result-cache, result-store,
      api-server) to a test case
- [x] 7.2 `golangci-lint` and `go test -race ./...` clean
- [ ] 7.3 Live check against `api.scorecard.dev` via `scorecard-mcp` — PENDING (not
      run in this session)

## 8. Documentation

- [x] 8.1 README: fallback env vars, the two flags/modes, max-age, and caveats
- [x] 8.2 Note in `docs/upstream-graft.md` that the tier is incubator-specific

## 9. Change closeout

- [x] 9.1 `openspec validate add-upstream-fallback --strict` passes
- [ ] 9.2 Archive the change once implemented and merged
