# Initial graft of the batch pipeline from ossf/scorecard

This tree was imported from [`ossf/scorecard`](https://github.com/ossf/scorecard),
not written here. This file is the record of that import: what came across, what
could not, and where to look for the history of the parts that could not.

## What the graft did

Extracted with `git filter-repo` from `ossf/scorecard` `main` @ `d1fab88f`, then
merged with `git merge --allow-unrelated-histories`.

| | |
|---|---|
| Imported commits | 466 non-merge, plus 2 retained 2020-era merges |
| History span | 2020-11-10 → 2026-08-07 |
| Files | 131 — 128 from `cron/`, 3 from the relocated token server |
| Tags imported | none (upstream's 48 release tags were stripped first) |

Three things were done to the history during extraction:

- `clients/githubrepo/roundtripper/tokens/server/` was relocated to
  `cron/internal/githubserver/`. Its parent `tokens/` package stayed upstream —
  it is imported by Scorecard's own round-tripper.
- Pull-request references in commit messages were rewritten from `(#N)` to
  `(ossf/scorecard#N)`, so they resolve to the repository they were opened
  against rather than auto-linking to unrelated items here. 394 commits carry a
  rewritten reference; the rest predate the squash-merge convention.
- Import paths were **not** rewritten across history. Historical commits contain
  the original `github.com/ossf/scorecard/v5/cron/...` paths, as authored. The
  rewrite to this module's path is a single commit at the tip.

`git log --follow` and `git blame` both work through the graft. Blame attributes
to original authors, not to the merge commit.

## Why the rest of this file exists

The pipeline's source, Dockerfiles, Cloud Build configs, and Kubernetes manifests
all came across with history. Its **build and CI wiring did not**, and that wiring
was rewritten by hand. What follows is the trail that rewriting would otherwise
have erased.

`git filter-repo` selects content by **path**, not by hunk. The build wiring could
not come across because it lives in files *shared with the scan engine*:

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
