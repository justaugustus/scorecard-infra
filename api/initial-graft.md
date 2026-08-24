# Initial graft of the results API from ossf/scorecard-webapp

This tree was imported from
[`ossf/scorecard-webapp`](https://github.com/ossf/scorecard-webapp), not written
here. This file is the record of that import: what came across, what could not,
and where to look for the history of the parts that could not.

It is the API's counterpart to [`cron/initial-graft.md`](../cron/initial-graft.md),
and it exists for the same reason: hand-porting erases provenance, so the
provenance gets written down next to the code it explains.

## What the graft did

Extracted with `git filter-repo` from `ossf/scorecard-webapp` `main` @
`39c800b`, then merged with `git merge --allow-unrelated-histories`.

| | |
| --- | --- |
| Imported commits | 150, no merge commits — the webapp squash-merges throughout |
| History span | 2021-12-29 → 2026-08-07, across 12 authors |
| Files | 117 — 116 under `api/`, plus `docs/dns.md` |
| Tags imported | none (upstream's 20 `v1.0.x` release tags were stripped first) |

Three things were done to the history during extraction:

- **Every path was renamed under `api/`.** Unlike the batch pipeline, the API was
  not namespaced upstream: `main.go`, `openapi.yaml`, `Makefile`,
  `Makefile.swagger`, `COPYRIGHT.txt` and `Dockerfile` lived at the webapp's
  repository root, and three of those collide with files here. The layout *inside*
  the tree is untouched, so `api/app/server/…` and `api/app/generated/…` stay 1:1
  with upstream while both copies exist.
- **`docs/dns.md` was imported unrenamed and then trimmed.** It documented DNS for
  both the Netlify website and the Cloud Run API, and a path filter cannot split a
  file. It came across whole; the website half was removed here and the API half
  is removed upstream, so both halves keep their history.
- **Pull-request references in commit messages were rewritten** from `(#N)` to
  `(ossf/scorecard-webapp#N)`, so they resolve to the repository they were opened
  against rather than auto-linking to unrelated items here. 140 of the 150
  commits carry a rewritten reference; the rest never had one.

Import paths were **not** rewritten across history. Historical commits contain
the original `github.com/ossf/scorecard-webapp/app/...` paths, as authored. The
rewrite to this module's path is a single commit at the tip.

`git log --follow` and `git blame` both work through the graft. Blame attributes
to original authors, not to the merge commit. `api/app/server/post_results_test.go`
resolves back through **two** renames — `app/signing_test.go` →
`app/post_results_test.go` → `app/server/post_results_test.go`.

## Why the rest of this file exists

The API's source, its OpenAPI contract, its generated tree, its Dockerfile and
its own Makefile all came across with history. Its **linter configuration and CI
wiring did not**, and both were rewritten by hand. What follows is the trail that
rewriting would otherwise have erased.

Unlike the pipeline's case, the obstacle was mostly *not* that these files are
shared with something that stayed behind. `main.yml` is entirely API content and
could have been imported. The reasons for hand-porting differ per file:

| File | Why it did not come across |
| --- | --- |
| `.golangci.yml` | `golangci-lint` reads **one** config, the nearest to the working directory. A second one under `api/` would be present, plausible-looking, and silently ignored. Its rules were reconciled into the repository-root config instead. |
| `.github/workflows/main.yml` | Entirely API content, but duplicates `presubmits.yml`'s build, test and lint on the same triggers. Importing it would have produced duplicate required checks, not coverage. Only its delta was ported. |
| `.github/workflows/cifuzz.yml` | Deferred, not rejected — see below. |
| `.github/workflows/codeql-analysis.yml` | Genuinely shared: a `go` and a `javascript` matrix. The JavaScript half belongs to the website. |
| `.github/dependabot.yml` | Shared across the Go module, the npm site, actions and Docker. |
| `.gitignore` | Shared; the API's single entry (`scorecard-webapp`) was re-added here as `/api/scorecard-webapp`. |
| `go.mod` / `go.sum` | Deliberately not imported. The API is now part of this module, and its 21 direct requirements were merged into the root `go.mod` at their upstream-pinned versions. |
| `README.md` | Shared, and describes a repository that still exists. |

## Provenance of the ported wiring

Commits are in `ossf/scorecard-webapp`. All hashes were resolved against `main` @
`39c800b` (2026-08-24).

| Wiring | Commit | Date | Subject | Commits touching it |
| --- | --- | --- | --- | --- |
| `main.yml` — build, lint, test, and the `addlicense` header check | `a307c5e` | 2022-06-01 | Add CI for linter, license and build (#103) | 56 |
| `.golangci.yml` | `c128cfc` | 2022-06-01 | Code cleanup (#102) | 10 |
| `cifuzz.yml` | `49c6e1a` | 2022-11-19 | Create CIFuzz Github action (#262) | 5 |
| `dependabot.yml` — the `gomod` ecosystem entry | `e8c4dbe` | 2022-01-10 | Create dependabot.yml (#7) | — |
| `.gitignore` — the `scorecard-webapp` binary entry | `4d36185` | 2021-12-29 | Skeleton to setup scorecard.dev webapp (#1) | 5 |

Note that `a307c5e` and `c128cfc` landed on the same day: CI and the linter
config arrived together, six months after the repository was created and two
months after the `GET` endpoint. The API ran without either for its first
several months, which is worth knowing before treating an early commit's style
as intentional.

To see the full history of any of them, in a clone of `ossf/scorecard-webapp`:

```console
git log --follow -- .github/workflows/main.yml
git log -p -S'addlicense' -- .github/workflows/main.yml
```

## What was ported, and how it differs

- **Linter configuration.** Upstream ran `default: none` with an explicit enable
  list. This repository's config descends from `ossf/scorecard`'s and enables
  strictly more, which surfaced 41 findings on import — all attributable to that
  difference rather than to defects, and all in hand-written source rather than
  the generated tree. The imported tree's own historical linter set is applied to
  `api/` only, in a block marked for removal when the behavior freeze lifts.
  **`govet` was deliberately kept enabled**, and it immediately caught a real
  defect: a non-constant format string in `api/main.go` that made the package fail
  to build under `go test`. Upstream never saw it because its CI only ever ran
  `cd app/server && go test`, never the root package.
- **CI delta.** `presubmits.yml` already did build, test and lint. What it gained
  from upstream is the `addlicense` header check (scoped to `api/`) and a GitHub
  token in the test environment, which the end-to-end specs need.
- **Image build.** `api-docker` joined `build-images.yml`'s matrix. The Dockerfile
  itself came across with history but needed two path fixes, because the module
  root is now two levels above it.
- **Fuzzing.** `cifuzz.yml` is **not** ported yet. The OSS-Fuzz project
  `scorecard-web` names the source repository in its build configuration in
  `google/oss-fuzz`, so porting the workflow without repointing the project would
  leave fuzzing running against the old source. Both are deferred together.

## Caveats

- **`git log -S` finds when a string first appears, not when a concept
  originated.** Check for earlier lineage under other names before concluding a
  job is as young as its pickaxe result suggests.
- **Ported CI is not a verbatim copy.** It was rewritten for this repository's
  paths and merged into workflows that already existed. Treat the upstream commits
  as the reason a check exists, not as a description of the current file.
- **This table is a point-in-time snapshot** taken during the migration. It is not
  maintained against upstream drift — but note that unlike the pipeline's case,
  its subject has *not* stopped changing: `ossf/scorecard-webapp` continues to
  exist and to host the website, so `main.yml`, `dependabot.yml` and
  `codeql-analysis.yml` will keep evolving there for the site's sake.
- **Upstream's release tags must not be deleted.** They were stripped from *this
  import* so they would not pollute this repository's namespace, which is the
  opposite of retiring them: `jetstack/tally` pins
  `github.com/ossf/scorecard-webapp v1.0.5`, and that resolution path has to keep
  working after the code is removed upstream.
