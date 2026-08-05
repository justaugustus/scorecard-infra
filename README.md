<!--
Copyright 2026 The uwu-tools Authors.
SPDX-License-Identifier: Apache-2.0
-->

# scorecard-api

A cloud-agnostic, self-hosted [OpenSSF Scorecard](https://github.com/ossf/scorecard)
results **API server**. It serves pre-computed Scorecard results from any object
store and **generates them live on demand** when the cache misses — a read-through
cache over an in-process scan engine ("hybrid").

It speaks the [`ossf/scorecard-webapp`](https://github.com/ossf/scorecard-webapp)
GET contract, so it is a drop-in `--base-url` target for
[`uwu-tools/scorecard-mcp`](https://github.com/uwu-tools/scorecard-mcp) and any
client of the public `api.scorecard.dev`.

> **Scorecard results are heuristic signals, not a verdict.** This server never
> asserts that a repository "is secure" or "is insecure"; every response declares
> its source, freshness, and completeness. A score of `-1` means *inconclusive*,
> not failing. See [`/capabilities`](#get-capabilities).

## Why

The public Scorecard API only covers repositories that opted in via
`publish_results: true`, and its production stack is wedded to Google Cloud. Teams
that want results for **private** repos, for repos **not** in the weekly public
scan, or on **non-GCP** infrastructure have no first-class option. This server
fills that gap: it serves the same contract from **any** object store and computes
results on demand.

## Endpoints

| Method & path | Returns |
| --- | --- |
| `GET /projects/{host}/{org}/{repo}` | Latest result as canonical Scorecard **JSON2** |
| `GET /projects/{host}/{org}/{repo}?commit={sha}` | The immutable result for a 40-hex commit |
| `GET /projects/{host}/{org}/{repo}/badge` | SVG badge for the aggregate score |
| `GET /capabilities` | This server's mode, coverage, freshness policy, and caveats |
| `GET /health` | Liveness (always `200` while serving) |
| `GET /readyz` | Readiness (`503` until dependencies are usable) |

`host` includes the TLD (e.g. `github.com`). `github.com` and `gitlab.com` are
supported.

On a cache miss the server attempts a synchronous scan within
`SCORECARD_SYNC_TIMEOUT`. If it finishes in time you get `200` with the result;
otherwise you get `202 Accepted` with a `Retry-After` header while the scan
continues in the background and populates the cache for your next request.
Commit-pinned results are immutable, so retries are cheap.

### Provenance headers

Result bodies are canonical JSON2, served verbatim so webapp-compatible clients
parse them unchanged. Provenance is carried in response headers instead:

| Header | Meaning |
| --- | --- |
| `X-Scorecard-Source` | `cached` or `live` |
| `X-Scorecard-Resolved-Commit` | The commit the result was computed at |
| `X-Scorecard-Generated-At` | Result generation date (RFC 3339) |
| `X-Scorecard-Version` | Scorecard engine version |
| `X-Scorecard-Complete` | Whether every check produced a conclusive score |

### `GET /capabilities`

Exists so clients report provenance from the server instead of assuming
public-cache behavior:

```json
{
  "mode": "cached+live",
  "checks": "all",
  "requires_opt_in": false,
  "latest_ttl_seconds": 86400,
  "caveats": [
    "Scorecard results are heuristic signals, not a verdict; a repository is never labeled secure or insecure.",
    "A score of -1 is inconclusive, not a failing score.",
    "Results are generated on demand for any repository the configured token can access; no publish_results opt-in is required.",
    "Latest results are cached with a TTL and refreshed on expiry; pin a commit for an immutable result."
  ]
}
```

## Storage

Results are persisted through [`gocloud.dev/blob`](https://gocloud.dev/), so the
backend is selected entirely by a URL — nothing cloud-specific is compiled in.
Credentials resolve via each backend's default chain.

| Backend | `SCORECARD_RESULTS_BUCKET_URL` example |
| --- | --- |
| Local filesystem | `file:///var/lib/scorecard` |
| S3 / MinIO / any S3-compatible | `s3://my-bucket?region=us-east-1&endpoint=localhost:9000&s3ForcePathStyle=true` |
| Azure Blob | `azblob://my-container` |
| Google Cloud Storage | `gs://my-bucket` |
| In-memory (tests only) | `mem://` |

Object keys match `scorecard-webapp` exactly, so the same objects are servable by
the public webapp and readable by `scorecard-mcp`:

```
{host}/{org}/{repo}/results.json            # latest (mutable, TTL)
{host}/{org}/{repo}/{commit}/results.json   # pinned (immutable)
```

## Configuration

All configuration comes from the environment. Only the bucket URL is required.

| Variable | Default | Description |
| --- | --- | --- |
| `SCORECARD_RESULTS_BUCKET_URL` | — (**required**) | `gocloud.dev/blob` URL of the result store |
| `SCORECARD_LISTEN_ADDR` | `:8080` | HTTP listen address (falls back to `:$PORT`) |
| `SCORECARD_LATEST_TTL` | `24h` | Freshness window for `latest` results |
| `SCORECARD_SYNC_TIMEOUT` | `20s` | How long a request waits before returning `202` |
| `SCORECARD_SCAN_TIMEOUT` | `5m` | Bound on a background scan |
| `SCORECARD_RETRY_AFTER` | `10s` | `Retry-After` hint on a `202` |
| `SCORECARD_SCAN_CONCURRENCY` | `4` | Max simultaneous live scans |
| `SCORECARD_ENABLED_CHECKS` | all | Comma-separated check names to restrict to |
| `SCORECARD_GITHUB_TOKENS` | — | Comma-separated SCM token pool (falls back to `GITHUB_AUTH_TOKEN`) |
| `SCORECARD_HOST_RATE_PER_SECOND` | `0` (unlimited) | Per-host scan rate limit |
| `SCORECARD_HOST_RATE_BURST` | `1` | Per-host rate burst |
| `SCORECARD_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

Live scans call SCM and Scorecard's auxiliary data sources, so they need network
egress and an SCM token (`GITHUB_AUTH_TOKEN`, or `SCORECARD_GITHUB_TOKENS` which
is fed into Scorecard's token rotation). Serving already-cached results needs
neither.

## Quick start

```sh
go build -o scorecard-api ./cmd/scorecard-api

export SCORECARD_RESULTS_BUCKET_URL="file:///tmp/scorecard"
export GITHUB_AUTH_TOKEN="ghp_..."   # required only for live scans
./scorecard-api
```

Query it:

```sh
# Latest (cache HIT if present, else a live scan populates the cache)
curl -s http://localhost:8080/projects/github.com/ossf/scorecard | jq .score

# A specific, immutable commit
curl -s "http://localhost:8080/projects/github.com/ossf/scorecard?commit=<40-hex-sha>"

# Badge, capabilities, health
curl -s http://localhost:8080/projects/github.com/ossf/scorecard/badge
curl -s http://localhost:8080/capabilities | jq .
curl -sI http://localhost:8080/health
```

### As a `scorecard-mcp` backend

```sh
scorecard-mcp --base-url http://localhost:8080 get_repo_score github.com/ossf/scorecard
```

## Development

```sh
go build ./...
go test ./... -race
golangci-lint run ./...        # config in .golangci.yml (aligned with ossf/scorecard)
```

An S3-compatible integration test runs when `SCORECARD_TEST_S3_URL` is set (e.g. a
local MinIO), and is skipped otherwise.

See [`AGENTS.md`](AGENTS.md) for conventions and [`docs/bootstrap.md`](docs/bootstrap.md)
plus the OpenSpec change under [`openspec/`](openspec/) for the full design.

## Architecture

```
cmd/scorecard-api/   the binary (config -> store + scanner -> orchestrator -> HTTP)
internal/
  model/         JSON2 result mirror + provenance + repo-ref parsing
  store/         gocloud.dev/blob store; backend by URL; the key contract
  orchestrator/  read-through cache: freshness/TTL, single-flight, sync/async
  scan/          wraps pkg/scorecard.Run; reused clients; worker pool
  tokens/        SCM token pool + per-host rate limiter + backoff
  httpapi/       the webapp GET contract, /capabilities, /health, /readyz
  config/        env-driven configuration
```

This is an incubator, not a permanent fork. The durable pieces are structured to
graft upstream over time — the contract + blob read path into `ossf/scorecard-webapp`,
and the live scan + HTTP surface into `ossf/scorecard`'s `scorecard serve`.

## License

[Apache-2.0](LICENSE).
