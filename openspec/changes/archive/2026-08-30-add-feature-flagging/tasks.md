# Tasks: Feature flagging via OpenFeature

## 0. Pre-work / decisions

- [x] 0.1 Confirm the OpenFeature Go SDK module + version and an in-process
      static/environment provider — `github.com/open-feature/go-sdk` v1.17.2 with
      the built-in `openfeature/memprovider`, seeded from env at startup (FF1/FF2)
- [x] 0.2 Confirm the flag key/type conventions — dot-scoped keys, bool/string,
      a capability owns/declares its own flags via `Definition` (FF6)

## 1. Dependencies

- [x] 1.1 Add `github.com/open-feature/go-sdk` (v1.17.2); default path uses only
      the SDK + in-process `memprovider` (no network client)

## 2. Flag seam (`internal/flags`)

- [x] 2.1 Implement the seam: `New` sets the provider and exposes typed `Bool`/
      `String` accessors taking an in-code default (FF5)
- [x] 2.2 Implement the in-process static/environment provider (default), seeding
      `memprovider` from env; no network I/O (FF2)
- [x] 2.3 Fail-safe evaluation: any evaluation error returns the caller's default (FF4)
- [x] 2.4 Test override: `New` takes a `getenv` func + per-instance `Domain`, so
      consumers set flag values in tests without touching global state

## 3. Configuration

- [x] 3.1 Add `SCORECARD_FLAGS_PROVIDER` (default `static`) to `internal/config`;
      `flags.New` (wired at startup in `cmd/scorecard-api`) validates it and fails
      fast on an unknown provider
- [x] 3.2 Validate static flag values at startup: `New` parses each env override
      per its declared kind and fails fast on an invalid value (FF7)

## 4. Testing

- [x] 4.1 Unit-test the seam: value present, unknown key → default, provider error
      → default (fail-safe), typed string/bool paths, explicit env-name override
- [x] 4.2 Unit-test provider selection (default + unknown) and startup validation
      (invalid bool, disallowed string)

## 5. Documentation

- [x] 5.1 Document the flags-vs-configuration convention (FF3) in AGENTS.md
- [x] 5.2 README: add `SCORECARD_FLAGS_PROVIDER` to the configuration table

## 6. Change closeout

- [x] 6.1 `openspec validate add-feature-flagging --strict` passes
- [x] 6.2 Archive the change once implemented and merged
