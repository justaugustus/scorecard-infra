# Tasks: Report the results API's build at `/version`

## 1. Contract and handler

- [x] 1.1 Add `/version` and `VersionInfo` to `api/openapi.yaml`; regenerate
      with the pinned go-swagger (**I2**).
- [x] 1.2 `api/app/server/version.go`: link-time `gitVersion`/`gitCommit`,
      parser version from build info, computed once (**I3**).
- [x] 1.3 Wire `GetVersionHandler` in `configure_scorecard.go`.

## 2. Build

- [x] 2.1 `api/Dockerfile` accepts `GIT_VERSION` and `GIT_HASH`;
      `api/Makefile` links them with `-X` ahead of `-extldflags` (**I3**).

## 3. Documentation

- [x] 3.1 `api/README.md`: the route, and drop the stale gateway wording
      about `openapi.yaml`.
- [x] 3.2 `deploy/api/README.md`: check `origin.scorecard.dev/version` after
      an apply.

## 4. Verification

- [x] 4.1 Unit tests: release build, nothing injected, replaced parser,
      parser not linked, no build info; the handler's headers and real
      parser.
- [x] 4.2 Built with `api/Makefile` and injected values, ran it, and queried
      `/version`: injected release and commit, `actionlint.kjanat.dev
      v1.17.0`, and the three cache headers. Rebuilt without the values:
      both read `unknown`.
- [ ] 4.3 After merge, confirm `main`'s published `scorecard-api` image
      carries the commit (a local Docker build couldn't reach Docker Hub).
- [ ] 4.4 After the next release is deployed, `origin.scorecard.dev/version`
      reports it.
