# Upload Testing Strategy

This project needs upload changes to be covered by deterministic automated tests, not by manual browser checks alone.

## Goal

Any change that touches upload behavior must prove:

- browser upload still works
- `Local Files` upload still works
- both paths keep the same backend semantics unless a difference is intentional and documented
- transient Telegram failures do not silently regress
- cancellation, cleanup, and progress tracking remain correct

## Test Layers

### 1. Backend unit and component tests

Primary packages:

- `internal/telegram`
- `internal/service`
- `internal/handler`
- `internal/localfs`

These tests should cover:

- Telegram client retry logic for:
  - `getFile` request errors
  - `getFile` body read errors
  - file download request errors
  - file body read errors
  - 429 handling
- upload verification behavior:
  - successful round-trip verification
  - verification mismatch cleanup
  - retry of transient chunk download failures
  - capped verification parallelism
- handler parity:
  - browser `Upload`
  - `LocalUpload`
  - request lifetime semantics
  - progress tracker setup and cleanup
- local filesystem safety:
  - traversal rejection
  - symlink escape rejection
  - directory expansion behavior

Implementation rule:

- network behavior must be simulated with `httptest` servers or custom `RoundTripper`s
- tests must assert terminal behavior, not only log messages

### 2. Frontend behavior tests

Primary files:

- `ui/src/pages/Files/operations.test.js`
- `ui/src/common/use_upload_manager.test.js`
- `ui/src/api/index.test.js`

These tests should cover:

- browser upload pipeline semantics
- `Local Files` upload pipeline semantics
- conflict resolution
- bulk upload ordering
- cancellation propagation
- storage-scoped local file API calls

Critical parity rule:

- if browser and `Local Files` intentionally use different queueing policies, that difference must be explicit in tests

### 3. Upload-path parity tests

For every upload-affecting change, add or update at least one test in this matrix:

| Scenario | Browser Upload | Local Files | Expected |
|---|---|---|---|
| single file success | yes | yes | uploaded and verified |
| 3-file batch success | yes | yes | all complete |
| transient `getFile` failure | yes | yes | retry then complete |
| transient file read failure | yes | yes | retry then complete |
| verification mismatch | yes | yes | upload fails and cleanup starts |
| cancellation during upload | yes | yes | upload stops and tracker settles |
| skip-on-conflict | yes | yes | skipped without upload |

Not every row needs a single end-to-end test, but every row must be covered somewhere in automated tests.

### 4. Smoke tests in CI or dev containers

Keep one higher-level smoke suite for upload flows using a fake Telegram server.

Target behavior:

- start app services
- stub Telegram API responses deterministically
- exercise:
  - browser upload endpoint
  - local upload endpoint
  - upload progress polling
- assert final API responses and DB-visible effects

This suite should stay small and stable. It is for path parity, not for exhaustive branch coverage.

## Definition Of Done For Upload Changes

An upload-related change is not done until:

1. The affected backend package tests pass.
2. The affected UI tests pass.
3. At least one upload-path parity scenario is added or updated when behavior changes.
4. No upload-specific behavior is validated only manually.

## Recommended Commands

Full repo:

```bash
make test
```

Focused upload suite:

```bash
make test-uploads
```

Local direct commands:

```bash
env GOCACHE=/tmp/pentaract-cli-gocache go test ./...
cd ui && node --test "src/**/*.test.js"
```

## Near-Term Gaps To Close

- add deterministic service-level tests for multi-file local uploads under transient Telegram failures
- add parity tests that compare browser upload and `Local Files` upload terminal outcomes for the same scripted Telegram error sequence
- add one smoke test suite that runs against a fake Telegram server instead of the real API
