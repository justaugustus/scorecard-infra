<!--
Copyright 2026 OpenSSF Scorecard Authors.
SPDX-License-Identifier: Apache-2.0
-->

# Results API

The service behind `api.scorecard.dev` and `api.securityscorecards.dev`,
imported from [`ossf/scorecard-webapp`](https://github.com/ossf/scorecard-webapp)
with its full history. **This is the API that ships.**

For what it is and what it serves, see
[Results API](../README.md#results-api-apiscorecarddev) in the repository
README. This document is the contributor's half: layout, build, and the notes
that save you an hour.

## Layout

| Path | Contents |
| --- | --- |
| `api/openapi.yaml` | The published contract. Also the API gateway's deployment configuration, so editing it changes a deployed service. |
| `api/app/server/` | Hand-written handlers: results retrieval, badge redirect, the signed-upload publish path, workflow verification, CDN purge. |
| `api/app/generated/` | go-swagger output derived from the contract. `configure_scorecard.go` is hand-owned despite living here. |
| `api/main.go` | Entry point. Builds as `scorecard-webapp` — see the note below. |
| `api/initial-graft.md` | What the import brought across, what it could not, and why. |

## Build

Run from the repository root:

```sh
make api-build     # build the binary (api/scorecard-webapp)
make api-swagger   # regenerate server and client from api/openapi.yaml
make api-docker    # build the container image
```

The image is published to `ghcr.io` by `publish-api-image.yml` on an `api/v*`
tag. Deploy the digest it reports, not a tag.

For the deployment itself — the AWS serving environment, its OpenTofu, and the
runbook — see [`deploy/api/README.md`](../deploy/api/README.md).

## One thing that looks like a mistake and is not

**The binary is named `scorecard-webapp`** while this repository's own server
binary is `scorecard-api` — which inverts what each one actually is. Renaming
changes image contents, so it is deferred: it does nothing for the migration and
costs a rebuild of the artifact every consumer is served from.

The tree used to hardcode `gs://` bucket URLs, which was the other entry here.
That is fixed — they are now `SCORECARD_RESULTS_BUCKET_URL` and
`SCORECARD_CRON_RESULTS_BUCKET_URL`, defaulting to the values that had been
compiled in, and the S3 driver is linked in alongside GCS.
