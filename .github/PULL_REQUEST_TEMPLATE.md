<!--
Thanks for contributing to scorecard-infra!

Before you open this PR, please make sure:
- Commits are signed off (DCO): `git commit -s` (see CONTRIBUTING.md).
- `make build`, `make test`, and `make lint` all pass.
- Specs and code stay in sync: if behavior changed, the OpenSpec change under
  openspec/ was updated too.

Adding a repository to the weekly scan? You only need the inventory section of
the checklist below — see CONTRIBUTING.md#adding-repositories-to-the-weekly-scan.
-->

## What this changes

<!-- A clear, concise description of the change and the motivation. -->

## Related

<!-- Link issues, the OpenSpec change (e.g. openspec/changes/...), or design
     decisions (e.g. design D5, C11) this PR implements. -->

## How it was tested

<!-- Commands run and their results; new/updated tests; manual verification. -->

## Checklist

- [ ] Commits are signed off (`git commit -s`)
- [ ] `make build` and `make test` pass
- [ ] `make lint` is clean
- [ ] Specs/docs updated if behavior changed
- [ ] No employer/internal references added

If this PR touches the **batch pipeline** (`cron/`):

- [ ] Runtime behavior is unchanged — the tree is behavior-frozen until cutover
- [ ] No new import edges between `cron/` and `internal/` in either direction
- [ ] If `cron/internal/format` changed, the corresponding engine change exists
      upstream and `schema_gen_test.go` passes

If this PR touches the **scan inventories** (`cron/internal/data/*.csv`):

- [ ] `make add-projects` was run and its output committed
- [ ] `make validate-projects` passes
