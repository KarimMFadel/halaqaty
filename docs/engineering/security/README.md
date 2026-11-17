# Security audits

Durable, in-repo record of security audit runs against this codebase. Each audit gets one dated file here; this README indexes them and records how results are produced so future scans stay comparable.

## Index

| Date | Report | Profile | Result |
|---|---|---|---|
| 2026-11-17 | [AUDIT-2026-11-17-run1.md](AUDIT-2026-11-17-run1.md) | standard, source-only, first run | 24 findings: 17 confirmed (5 medium / 12 low), 7 needs_validation; 0 critical / 0 high |

## How audit results are produced (method provenance)

1. **Tooling:** the local `security-audit` skill (OpenCode, `security-audit` profile) drives the run; subagent hunters, coverage critics, and an independent verification pass execute it.
2. **Workflow:** architecture map → coverage ledger (`coverage-ledger.json`) → hunter waves with per-unit owned evidence → post-wave coverage critic (must return `stop`) → distinct final-clean critic → findings consolidation (`findings.json`) → independent verification pass (per-finding anchor re-derivation) → final report.
3. **Validation gates:** machine-checked with the skill's validators — `validate-coverage-ledger.cjs` and `validate-findings.cjs` (run via `docker run node:22-alpine`). A run is only recorded here when both validators PASS.
4. **Execution boundary (Windows host):** no target-controlled dynamic execution; confirmed verdicts rest on source-enumeration proofs. Candidates needing runtime/hosted facts stay `needs_validation` until validated in a proper sandbox or by the repo owner.
5. **Raw artifacts (not committed):** each run keeps its full detail set — coverage ledger, findings JSON, bookkeeping (candidate fingerprints + hardening backlog), per-finding detail docs — in a local run folder outside this repo (e.g. `C:\Users\karim.fadel\security-audit-skill\halaqaty\run-<n>\`). Only this dated summary lands in the repo.
6. **Updating entries:** when a finding is fixed, change its `Status` column in the dated file (`open → fixed in <commit>`) in the same commit as the fix. When a needs_validation item is resolved, record the resolution. Never delete entries — history is the point.
