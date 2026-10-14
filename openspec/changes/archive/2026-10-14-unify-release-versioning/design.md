# Design: One release line for the whole repository

Decision IDs are **V\*** here, distinct from `migrate-batch-pipeline`'s **C\***,
`migrate-api`'s **W\***, `configure-result-buckets`' **B\***, and the
cloud-agnostic server's **D\***/**F\***.

## Context

`migrate-api` **W15** scoped tags to the component "from the first one" and gave
three reasons. This change reverses that choice, so each reason is answered
under **V1**. W15's other rule — the public API gets an explicit, nameable
release and is never deployed on merge — still holds, and nothing here weakens
it.

## Decisions

### V1 — One `v`-prefixed SemVer line for the repository

Tags are `vX.Y.Z`, matching the other Scorecard repositories. A tag names a
release of the whole repository, not of one component. W15's reasons, in turn:

- **"A bare version would claim the whole repository for one component."** It
  now claims the whole repository on purpose. Both systems build from the root
  `go.mod`, so a release of one is a release of the engine version both run.
- **"It would collide with module-level versioning of `scorecard-infra`."** This
  *is* module-level versioning. A future `api/` submodule would still take
  `api/vX.Y.Z` tags, which do not conflict with root tags.
- **"It would read ambiguously next to upstream's `v1.0.x`."** Those tags belong
  to a different module path, `github.com/ossf/scorecard-webapp`, and a v0 line
  does not overlap their range.

### V2 — Stay in v0, starting at `v0.2.0`

Under Go's module rules, v1 and above promise a compatible Go API, and breaking
it later needs a `/v2` module path. This repository's Go packages are not a
library anyone should depend on, so that promise buys nothing. Within v0:

- **minor** for breaking changes, features, and engine bumps;
- **patch** for fixes.

`v0.2.0`, not `v0.1.0`, signals that this is not the first release.

**Caveats.**

- **Tags become Go module versions, permanently.** The module proxy ignores
  `api/v1.1.0` today. A root `v0.x.y` tag is a real version: once fetched, it is
  recorded in the checksum database and cannot be moved or reused. A bad release
  is withdrawn with a `retract` directive, not by deleting the tag.
- **The number appears to go backwards** after `api/v1.1.0`. The `v0.2.0`
  release notes say it supersedes `api/v1.1.0` and `cron/v*`, that it now covers
  the batch pipeline too, and why the line is v0.

### V3 — One tag publishes every image, from one workflow

A tag publishes all seven images (the API and the six pipeline images) from the
same commit. Deploys stay independent: each system deploys by digest, on its own
schedule.

One release is one workflow run, so tag derivation, the `:latest` rule, and the
labels exist once rather than drifting apart in two files.
`publish-cron-images.yml` is renamed with `git mv` to keep its history, and the
API joins its matrix. Its provenance step, which resolves the engine commit
through the module proxy, now covers the API too.

Merges to `main` still publish a mutable `main` tag for staging. A manual
dispatch's extra tag now applies to every image. Nothing consumes such a tag on
the pipeline images, so no image selector is added.

The tag trigger changes in the same step as the workflow layout, which gives a
failed first release two candidate causes. A pre-release tag pushed to a fork
exercises the whole workflow against the fork's own `ghcr.io` namespace before
`v0.2.0` is tagged.

### V4 — Pre-releases do not move `:latest`

A tag containing `-` (for example `v0.2.0-rc.1`) is a SemVer pre-release. It
gets its version tag and SHA tag but not `:latest`, so an rc cannot become what
`:latest` resolves to.

### V5 — The engine version is recorded, not encoded

The infra version does not track the engine's:

- **The schedules differ.** The
  [ossf/scorecard#5322](https://github.com/ossf/scorecard/issues/5322) fix
  ships between engine releases, and this repository has pinned engine
  pre-release builds.
- **No suffix works.** `v5.6.0-1` sorts before `v5.6.0` under SemVer, and `+1`
  is not allowed in image tags.
- **A `v5` tag claims major version 5** of this module, which would need a `/v5`
  module path.

Instead, every image carries OCI labels:

| Label | Value |
| --- | --- |
| `org.opencontainers.image.version` | The release tag, or `git describe` on a branch build |
| `dev.scorecard.engine.version` | `github.com/ossf/scorecard/v5`'s version in `go.mod` |
| `dev.scorecard.engine.commit` | That version's commit, from the module proxy |

`org.opencontainers.image.*` is reserved for keys OCI predefines, and
`image.version` is "version of the packaged software", which is this
repository's release. Custom keys use the reverse DNS of the project's own
domain, `scorecard.dev`. "Engine" is the term the Makefile and the provenance
package already use. For the API the labels matter most: its binary embeds no
engine commit, so until a version endpoint exists they are the only
machine-readable record of what it runs.

## Open questions

- **Commit the production digest.** The API's production `image` is passed at
  `tofu apply` time and committed nowhere, and production was found running a
  `main` build rather than a release image. Committing it would make every
  deploy a reviewable change. Separate change.
