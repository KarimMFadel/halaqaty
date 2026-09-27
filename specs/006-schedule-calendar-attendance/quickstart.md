# F-006 Planning Quickstart

1. Work on `006-schedule-calendar-attendance`. In the same PowerShell process as each Spec-Kit script, set `$env:SPECIFY_FEATURE_DIRECTORY = 'specs/006-schedule-calendar-attendance'`; run `.specify/scripts/powershell/check-prerequisites.ps1 -Json -PathsOnly` and confirm the feature directory.
2. Review [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [the REST contract](contracts/schedule-calendar.openapi.yaml), [the additive profile contract](contracts/profile-timezone.md), ADR-025, and accepted ADR-026. F-006 is approved; F-008 remains Proposed.
3. Use [tasks.md](tasks.md) to track the remaining migration, handler, test and review work. ADR-026 is accepted, `docs/engineering/architecture/ARCHITECTURE.md` reflects it, and both contract changes are synchronized into `docs/contracts/openapi.yaml`. The approved contract does not mean the endpoints or migration already exist.
4. Run `/speckit.analyze` against the current spec, plan, tasks, canonical contract and ADR status. Only then request `/speckit.implement` for approved work. A partial pilot must visibly report SC-009 pending F-008.
5. During implementation, test recurrence/DST fixtures, transaction races, role denial, attendance threshold and manual corrections with focused tests; then run the unfiltered repository gates and real mobile/backend integration checks. A skipped environment-dependent check is unverified, not passing.

The feature contract is a proposal, not a running endpoint. No curl sample is provided until it matches an implemented handler and the canonical OpenAPI contract.
