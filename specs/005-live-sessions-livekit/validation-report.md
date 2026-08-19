# F-005 Validation Report

Updated: 2026-08-19

## Passed

- Backend short suite: `go test -short ./...`.
- Recovery contract subset: `go test -tags=contract ./tests/contract -run 'TestLiveSessionsReconnect' -count=1`.
- LiveKit boundary contract subset.
- Observability/rate-limit integration subset (isolated Go cache).
- Flutter session widget suite: 31 tests.
- Flutter changed-file analyze and Dart format.
- Canonical OpenAPI Spectral lint.

## Blocked or open

- T041 recovery integration execution is blocked by the local Go installation reporting `package bytes is not in std` before integration tests compile.
- Flutter integration/device gates were unavailable in this environment.
- Full contract suite retains the pre-existing `TestModerationWebSocketResponseSafetyContract` authorization mismatch.
- T043 remains open pending atomic start locking through provider credential issuance/admission; missing-room normalization is implemented and covered by an adapter test.
