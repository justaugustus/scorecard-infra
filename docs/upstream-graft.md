# Where the code goes: consolidation, and what still grafts upstream

This document used to describe an incubator. It said this repository held novel
glue that would graft outward — the blob read path and the `/projects` contract
to `ossf/scorecard-webapp`, the live-scan path and HTTP surface to
`ossf/scorecard`'s `scorecard serve` — and that the job was to structure code so
it would lift out cleanly.

**Two of those three graft targets are now here.** The batch pipeline arrived
from `ossf/scorecard` with its history; the results API arrived from
`ossf/scorecard-webapp` with its history. The code the incubator was meant to
improve on came to it. So the strategy inverted: consolidation replaced
grafting, and this repository is the unified home for Scorecard's
infrastructure rather than a staging area for pieces of it.

That is a change of direction, not a change of goal. The endgame is still one
server serving the published contract over a configurable backend, fed by a
producer that writes through the same store. What changed is that getting there
is now an in-repository convergence, reviewable as a diff, rather than a
negotiation across three repositories.

This is the detailed companion to design decision **W10** (and touches **D4**,
**D7**, **D13**, **C11**). The README and `AGENTS.md` point here.

## What lives here now

Four things, in two categories.

**Arrived to stay, behavior-frozen:**

| Tree | What it is | Provenance |
| --- | --- | --- |
| `cron/` | Batch scanning pipeline: PubSub controller, batch and CII workers, BigQuery transfer, release webhook, token-pool server, scan inventories | [`cron/initial-graft.md`](../cron/initial-graft.md) |
| `api/` | The results API serving `api.scorecard.dev`: `GET` results and badge, the Sigstore-verified `POST` publish path, and the `openapi.yaml` contract | [`api/initial-graft.md`](../api/initial-graft.md) |

**Built here:**

| Tree | What it is | Status |
| --- | --- | --- |
| `internal/`, `cmd/scorecard-api` | A provider-agnostic hybrid API server: `gocloud.dev/blob` store, read-through cache, live scan, token pool, `/capabilities` | Off the deployment path — see below |
| `docs/research/` | The provider-agnostic reference design | Proposal-flavored |

## Which server ships

`api/` and `internal/httpapi` both implement the results contract. **The imported
one is what deploys.**

The reasoning is not that it is better designed. It is that re-hosting running
infrastructure is a lift-and-shift, and between two implementations of one
contract, the one already proven against every live consumer — the website's
result viewer, `img.shields.io`, the Scorecard Action's upload path,
`scorecard-mcp` — wins by default. It wins especially during a migration whose
acceptance test is that nothing changed.

`internal/httpapi` and its supporting packages stay: they build, they keep their
tests, and they are where the provider-agnostic work was learned.
`internal/store`'s configurable-backend seam is still the most likely shape of
the eventual provider exit. But they are not the forward path today, and nobody
should add new API surface there expecting it to ship. The decision is meant to
stay cheap to revisit; it is not meant to be ambiguous.

A package nobody deploys and nobody has told keeps compiling while it quietly
stops being true. The honest options at the next decision point are to converge
it or to remove it — not to maintain it indefinitely as a study.

## What still grafts upstream

Narrower than this document used to claim, and worth being precise about so that
nobody structures code for a graft that is not going to happen.

| Piece | Target | Status |
| --- | --- | --- |
| `/capabilities` (**D7**) | A new endpoint upstream, plus a reader in `scorecard-mcp` | **Still live.** The MCP hardcodes public-cache provenance in `internal/provider/rest.go`, which is wrong for any other backend. This remains the clearest thing worth contributing outward. |
| The read-through cache seam (**D2/D5/D6**) | `scorecard serve`, as an optional blob cache | **Deferred behind convergence.** It cannot graft anywhere useful until there is one server here rather than two. |
| The `/projects` contract and blob read path | *was* `ossf/scorecard-webapp` | **Retired.** The destination is `api/`, in this repository. |
| The live scan path and HTTP surface | *was* `scorecard serve` | **Deferred.** `scorecard serve` still has no store and no cloud dependency; teaching it a cache is downstream of convergence here. |
| `cron/` | — | **Never grafted.** Direction of travel was inbound (**C6**/**C13**). |
| `api/` | — | **Never grafts.** Direction of travel was inbound (**W1**). |

## Rationales that stopped holding

Recorded rather than deleted, because a rationale that quietly expires is worse
than one that is argued with.

- **`internal/model` (D13)** was justified partly by the webapp's generated models
  living upstream, so an in-repo mirror avoided a dependency on them. They no
  longer live upstream — they are in `api/app/generated/models`. The mirror may
  still be worth keeping for its own reasons (it is lean, and it exists to make
  score and freshness introspectable), but the original argument is gone. Revisit
  in the convergence change.
- **"Keep `store` and the `/projects` handlers thin and faithful to the webapp so
  they lift out cleanly."** There is nothing to lift out to. Faithfulness to the
  webapp's contract still matters — it is the published contract — but the reason
  has changed from portability to compatibility.
- **"This repo is an incubator, not a permanent fork."** It was never a fork, and
  it is no longer an incubator. It is where the infrastructure lives.

## Rules while the frozen trees are frozen

These are prohibitions, and they hold until production cutover completes:

- **No import edges** between `api/`, `cron/`, and this repository's own packages,
  in any direction. Enforced by the `import-edges` job in `presubmits.yml`. This
  is not a claim that the trees are unrelated — it is what keeps each one
  byte-comparable to what production runs.
- **The hardcoded `gs://` constants in `api/` stay**, notwithstanding this
  repository's cloud-agnostic rules, which govern its own code. Making them
  configurable is a follow-on change, deliberately separate so the import stays a
  provable relocation.
- **`api/openapi.yaml` is not edited.** It is simultaneously the published
  contract and the API gateway's deployment configuration. Editing it changes a
  deployed service.

## References

- **W10** (which server ships, and why convergence is deferred) —
  `openspec/changes/migrate-api/design.md`
- **C11** (the same question for the pipeline) —
  `openspec/changes/migrate-batch-pipeline/design.md`
- **D4** (key/body contract), **D7** (`/capabilities`), **D13** (result model) —
  `openspec/changes/archive/2026-08-06-add-scorecard-api-server/design.md`
- Provenance of the two imports — [`cron/initial-graft.md`](../cron/initial-graft.md),
  [`api/initial-graft.md`](../api/initial-graft.md)
- `scorecard serve`: `github.com/ossf/scorecard` (`cmd`/`serve`)
- Reference client / acceptance test: `github.com/uwu-tools/scorecard-mcp`
