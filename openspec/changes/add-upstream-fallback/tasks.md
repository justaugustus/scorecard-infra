# Tasks: Upstream result fallback (api.scorecard.dev)

## 0. Pre-work / decisions

- [ ] 0.1 Confirm `api.scorecard.dev` serves the `/projects/{host}/{org}/{repo}`
      GET contract as canonical JSON2; capture a sample body and the status code
      for an unknown/opted-out repo (expect 404 = miss) (design F8)
- [ ] 0.2 Confirm blob metadata is supported across the store backends in use for
      the origin tag (design F6)

## 1. Configuration and flags

- [ ] 1.1 Add `SCORECARD_FALLBACK_URL`, `SCORECARD_FALLBACK_TIMEOUT` (default 5s),
      and `SCORECARD_FALLBACK_MAX_AGE` (default 7d) to `internal/config`; validate
      (URL parses; timeout/max-age > 0)
- [ ] 1.2 Declare `fallback.enabled` (bool, default true) and `fallback.mode`
      (string, default `fetch-first`, allowed {fetch-first, safety-net}) as
      `flags.Definition`s; wire them in `cmd/scorecard-api` and capture the flag
      client for the orchestrator (design F2)

## 2. Fallback client (`internal/fallback`)

- [ ] 2.1 Define the `Fallback` interface and `ErrFallbackMiss` (F1)
- [ ] 2.2 Implement the HTTP client: `GET {url}/projects/{host}/{org}/{repo}`,
      parse via `model.Parse`, 404/empty → miss, other errors → non-fatal; honor
      the timeout (F5/F8)
- [ ] 2.3 Enforce the max-age bound: a result older than max-age is a miss (F6)
- [ ] 2.4 Unit-test hit, miss (404), error, timeout, and stale-beyond-max-age

## 3. Model

- [ ] 3.1 Add `model.SourceUpstream = "upstream"` (F3)
- [ ] 3.2 Implement the uniform completeness rule (present AND conclusive), applied
      to every source; unit-test omitted-check and `-1` cases (F4)

## 4. Store origin tag

- [ ] 4.1 Persist/retrieve an origin tag (locally scanned vs. upstream) as blob
      metadata, leaving the JSON2 body untouched; untagged = locally scanned (F6)
- [ ] 4.2 Extend the store read path to return the origin alongside the body
- [ ] 4.3 Unit-test round-trip of the tag and the legacy-untagged default

## 5. Orchestrator integration

- [ ] 5.1 Add an optional `Fallback` + the flag client via options; read
      `fallback.enabled`/`fallback.mode` per request (F2)
- [ ] 5.2 Implement mode-aware ordering: fetch-first (upstream → scan) and
      safety-net (scan → upstream on terminal skip/error) (F3/F9)
- [ ] 5.3 Make freshness source-aware (TTL for local, max-age for upstream) and
      report the true source on store hits (F6)
- [ ] 5.4 Backfill a used upstream result (within max-age) tagged as upstream;
      bypass the upstream for commit-pinned requests; keep the fetch outside the
      singleflight/202 path (F5/F6/F7)
- [ ] 5.5 Unit-test with fake `Fallback` + flags: fetch-first hit/backfill,
      fetch-first miss→scan, safety-net scan-success (no upstream), safety-net
      skip→upstream, commit bypass, source-aware freshness, kill-switch off

## 6. HTTP surface / capabilities

- [ ] 6.1 Extend `/capabilities` to reflect the fallback mode and caveats when
      active (api-server delta)
- [ ] 6.2 Confirm `X-Scorecard-Source: upstream` flows through the provenance
      path; integration-test it

## 7. Testing and verification

- [ ] 7.1 Map each spec scenario (upstream-fallback, result-cache, result-store,
      api-server) to a test case
- [ ] 7.2 `golangci-lint` and `go test -race ./...` clean
- [ ] 7.3 Live check against `api.scorecard.dev`: fetch-first serves+backfills an
      opted-in repo; a non-opted-in repo misses and scans; verify capabilities and
      the source header via `scorecard-mcp`

## 8. Documentation

- [ ] 8.1 README: the fallback env vars, the two flags/modes, max-age, and the
      honesty caveats
- [ ] 8.2 Note in `docs/upstream-graft.md` that this tier is incubator-specific
      and not intended to graft upstream

## 9. Change closeout

- [ ] 9.1 `openspec validate add-upstream-fallback --strict` passes
- [ ] 9.2 Archive the change once implemented and merged
