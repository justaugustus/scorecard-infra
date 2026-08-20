# Provenance of the ported cron build wiring

This file records where the batch pipeline's build and CI wiring came from in
`ossf/scorecard`, because — unlike the pipeline's source, manifests, and
Dockerfiles — that wiring could not be imported with its history.

## Why this file exists

The migration used `git filter-repo`, which selects content by **path**, not by
hunk. Everything under `cron/` came across with full history. The build wiring
did not, because it lives in files that are *shared with the scan engine*:

| File | Cron content | Total |
|---|---|---|
| `Makefile` | 96 lines mention cron | 467 lines |
| `.github/workflows/docker.yml` | 6 of 8 `docker_matrix` targets | whole-repo image CI |
| `.github/workflows/main.yml` | `add-projects`, `validate-projects` | whole-repo CI |
| `.github/dependabot.yml` | 5 Dockerfile directory entries | whole-repo deps |
| `.codecov.yml` | one `cron/**/*` ignore entry | whole-repo coverage |
| `cloudbuild/README.md` | reference to cron image builds | engine doc |

There is no way to extract "the 96 cron lines of the Makefile" with a path
filter. It is the whole file or none of it, and the whole file is roughly 79%
engine build targets. Importing it to recover a handful of recipes would have
dragged the engine's entire build history along with it.

So those fragments were **ported by hand**. This file is the trail that porting
would otherwise have erased.

Note that `.ko.yaml` is *not* in the list. Upstream's contains a single
`scorecard` build id and nothing cron-related — the "cron ko targets" are
Makefile recipes invoking `ko build` against import paths, not entries in a ko
config. There was nothing to port from it.

## What was imported with history, for contrast

These all live inside `cron/` and carry their full commit history in this
repository. Use `git log --follow` on them directly; this file is not needed.

- 6 Dockerfiles — `bq`, `cii`, `controller`, `webhook`, `worker`, `githubserver`
- 6 Cloud Build configs — `cron/cloudbuild/*.yaml` and
  `cron/internal/githubserver/cloudbuild.yaml`
- 12 Kubernetes manifests — `cron/k8s/*.yaml`

## Provenance of the ported wiring

Commits are in `ossf/scorecard`. All hashes were resolved against `main` @
`d1fab88f` (2026-08-20).

### Makefile targets

The image targets reached their **current names** in one 2021 restructuring:

| Commit | Date | Subject |
|---|---|---|
| `aa558ff2` | 2021-12-02 | Add parallelism to improve build times (#1342) |

That commit introduced all twelve of `cron-{controller,worker,cii-worker,bq-transfer,webhook,github-server}-{docker,ko}`. It is a rename and
restructure, **not** the origin of cron building — the lineage starts earlier
under different target names:

| Commit | Date | Subject |
|---|---|---|
| `688dc5e6` | 2021-03-19 | :sparkles: Refactor cron job |
| `7622cea5` | 2021-03-22 | :seedling: updated the makefile to include scripts and cron |
| `eade3f95` | 2021-04-26 | :seedling: Included go mod verify for cron and scripts |
| `d3a59eac` | 2021-04-27 | Move Dockerfile.gsutil to inside cron/ |

25 commits in total touch cron content in the Makefile. To see them:

```console
git log -S'cron' --oneline -- Makefile          # in a clone of ossf/scorecard
```

The non-image targets have single, clean origins:

| Target | Commit | Date | Subject |
|---|---|---|---|
| `add-projects` | `c06f89af` | 2021-06-10 | Script to add new projects to projects.csv file (#567) |
| `validate-projects` | `ba3b5c59` | 2021-05-15 | Refactor Makefile and add proto compile support. (#458) |
| `build-proto` | `ba3b5c59` | 2021-05-15 | Refactor Makefile and add proto compile support. (#458) |

### Workflows and configuration

| Wiring | Commit | Date | Subject | Commits |
|---|---|---|---|---|
| `docker.yml` cron matrix entries | `35511342` | 2022-02-15 | :seedling: Parallelize the builds | 2 |
| `main.yml` `add-projects` / `validate-projects` jobs | `bba55d42` | 2022-02-16 | :seedling: Parallelize builds | 1 |
| `dependabot.yml` cron Dockerfile paths | `775a83a2` | 2021-03-22 | :seedling: update dependabot for cron and scripts | 4 |
| `.codecov.yml` cron ignore | `5656c3ed` | 2022-02-22 | :seedling: Ignore cron folder from codecov | 1 |
| `cloudbuild/README.md` cron reference | `f49aad68` | 2021-05-16 | :sparkles: Moved the cloudbuilds to yaml (#444) | — |

The `docker_matrix` cron entries as they stood at migration time:

```yaml
- 'cron-controller-docker'
- 'cron-worker-docker'
- 'cron-cii-worker-docker'
- 'cron-bq-transfer-docker'
- 'cron-webhook-docker'
- 'cron-github-server-docker'
```

(`scorecard-docker` and `build-attestor-docker` stay upstream — they are engine
targets and were never part of this migration.)

## Doing further archaeology

`ossf/scorecard`'s history is public and is not going away, so the full record
remains reachable even after the pipeline is deleted from that repository. In a
clone of it:

```console
git log -S'<target-or-job-name>' --oneline -- Makefile
git log -p -S'cron' -- .github/workflows/docker.yml
git log --follow -- cloudbuild/README.md
```

The one thing to remember is that deletion does not erase history: after the
removal lands, these commits are still in `ossf/scorecard`'s log, just not in its
working tree.

## Caveats

- **`git log -S` finds when a string first appears, not when a concept
  originated.** That is exactly why `aa558ff2` shows up for all twelve image
  targets — it renamed them. Always check for earlier lineage under other names
  before concluding a target is as young as its pickaxe result suggests.
- **Ported recipes are not verbatim copies.** They were rewritten for this
  repository's module path and root, and they no longer share the engine's
  Makefile variables. Treat the upstream commits as the reason a target exists
  and why it has particular flags, not as a description of the current file.
- **This table is a point-in-time snapshot** taken during the migration. It is
  not maintained against upstream drift, and it does not need to be — its
  subject stopped changing when the pipeline moved.
