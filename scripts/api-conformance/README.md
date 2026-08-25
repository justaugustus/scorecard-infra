# Results-API conformance harness

Compares two deployments of the results API and reports any request whose
observable behavior differs. This is the cutover gate from design **W11**,
step 3: the migrated service must answer identically to the one it replaces,
before traffic moves.

```sh
# The gate: query both, report differences.
./conformance.sh compare https://api.scorecard.dev https://candidate.example

# Point-in-time record of one deployment.
./conformance.sh capture https://api.scorecard.dev ./baseline
```

Requires `curl` and `jq`. The request set is [`requests.tsv`](requests.tsv);
add cases there rather than passing ad-hoc URLs, so a run is reproducible and
the same set is used before and after.

## What it compares

Status code, `Content-Type`, `Location`, the CORS headers, the caching headers
the CDN and browsers act on (`Cache-Control`, `Surrogate-Control`,
`Surrogate-Key`), and the response body with JSON normalized so formatting is
not mistaken for behavior.

Ignored, because they differ between any two deployments without the behavior
differing: `Date`, `Server`, `Alt-Svc`, `Via`, trace and request IDs,
`Content-Length`, `Set-Cookie`.

Recorded but never compared: `Age`, `X-Cache`, `X-Served-By`. These are printed
alongside any body difference, because cache vintage is the most common innocent
explanation for one — see below.

## Compare origins, not CDN hostnames

**A comparison between two CDN-fronted hostnames measures cache state, not
behavior.** This is not hypothetical. Comparing the two production hostnames,
which front the same service, reports a difference:

```
DIFF  results-known-good
  api.scorecard.dev            date 2026-08-22   age 251220
  api.securityscorecards.dev   date 2026-08-15   age 287665
```

Same repository, same resolved commit, same engine version — result bodies from
scans a week apart. `Surrogate-Control` is `max-age=31557600`, so an edge object
survives a year unless purged, and `post_results.go` purges a single
`API_BASE_URL`. A purge therefore refreshes one hostname and leaves the other
serving whatever it last cached.

Confirmed from the deployment rather than inferred: the Cloud Run service sets
`API_BASE_URL=https://api.scorecard.dev`, and only `api.securityscorecards.dev`
has an Endpoints service in front of it — so the two hostnames neither share a
purge nor reach the backend by the same path.

Two consequences:

1. **Point the harness at origins** — the Cloud Run URL, the candidate
   deployment's direct address — not at `api.scorecard.dev`. Otherwise a
   green run means "both caches happen to agree" and a red one means nothing in
   particular.
2. **This is a live defect in the current deployment**, independent of any
   migration: which hostname a consumer uses determines how fresh their results
   are. Worth fixing where the purge happens rather than carrying it across.

## What it does not cover

**The `POST` publish path.** It has side effects — verification against live
Sigstore, a write to the results bucket, a CDN purge — so it cannot be exercised
against production from a script. Confirming it is a separate cutover step:
watch a real Scorecard Action upload land end to end.

**Load, latency, and concurrency.** This checks that answers match, not that
they arrive fast enough or that the service survives production traffic.

## Interpreting a failure

A difference is a question, not automatically a defect. Work through it in this
order:

1. **Cache vintage** — check the printed `age` and `x-cache`. Different vintages
   of the same object are not a behavior difference. Re-run against origins.
2. **Different data** — compare `date`, `repo.commit`, and `scorecard.version`
   in the two bodies. If those differ, the deployments are reading different
   buckets or a bucket that is not fully replicated. That is a data-migration
   problem, not an API problem, and `--ignore-scan-metadata` will hide it —
   use that flag only when the divergence is known and accepted.
3. **Actual behavior** — a status code, header, or check-result difference that
   survives 1 and 2 is a real finding and blocks the traffic shift.

Exit status is 0 when nothing differs, 1 otherwise, so it can gate a script.
