# Tasks: Feature flagging via OpenFeature

## 0. Pre-work / decisions

- [ ] 0.1 Confirm the OpenFeature Go SDK module + version and an in-process
      static/environment provider (SDK-provided or a thin local one), matching the
      project's Go toolchain (design FF1/FF2)
- [ ] 0.2 Confirm the flag key/type conventions (dot-scoped keys, bool/string) and
      that a capability owns/documents its own flags (design FF6)

## 1. Dependencies

- [ ] 1.1 Add `github.com/open-feature/go-sdk` and the chosen in-process provider;
      keep the dependency surface minimal (no network client in the default path)

## 2. Flag seam (`internal/flags`)

- [ ] 2.1 Implement the seam: initialize the OpenFeature client, select the
      provider, and expose typed accessors (`Bool`/`String`) that take an in-code
      default (design FF5)
- [ ] 2.2 Implement the in-process static/environment provider (default); perform
      no network I/O (FF2)
- [ ] 2.3 Enforce fail-safe evaluation: on any evaluation error, return the
      caller's default (FF4)
- [ ] 2.4 Provide a fake/override for tests so consumers can set flag values

## 3. Configuration

- [ ] 3.1 Add `SCORECARD_FLAGS_PROVIDER` (default `static`) to `internal/config`;
      validate the value and fail fast on an unknown provider
- [ ] 3.2 Validate known static flag values at startup for the static provider
      (FF7), consistent with existing config validation

## 4. Testing

- [ ] 4.1 Unit-test the seam: value present, unknown key → default, provider error
      → default (fail-safe), and typed string/bool paths
- [ ] 4.2 Unit-test provider selection and startup validation (default + invalid)

## 5. Documentation

- [ ] 5.1 Document the flags-vs-configuration convention (FF3) in AGENTS.md so
      future changes classify new settings consistently
- [ ] 5.2 README: `SCORECARD_FLAGS_PROVIDER`, the default (static/offline), and how
      a capability registers a flag with a safe default

## 6. Change closeout

- [ ] 6.1 `openspec validate add-feature-flagging --strict` passes
- [ ] 6.2 Archive the change once implemented and merged
