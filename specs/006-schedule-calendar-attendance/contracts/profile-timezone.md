# F-006 additive profile contract

The existing canonical `GET /auth/me` and `PUT /auth/me` paths and methods remain unchanged. `docs/contracts/openapi.yaml` now specifies a `timezone` string on `UserProfile` and an optional `timezone` string on `UpdateProfileRequest`. Both require a valid IANA timezone name such as `Africa/Cairo`; the backend will reject unknown names with the existing validation envelope. `GET /auth/me` will return the stored value. `PUT /auth/me` will update it when supplied and return the resulting profile. Omitting it preserves the existing value. Existing profiles receive `UTC` in the additive migration, so old clients remain compatible and new clients have a deterministic default.

The calendar's requested month is interpreted in this stored viewer timezone. The planning timezone in `PlanInput` remains independent: it determines the recurrence clock, while the profile timezone determines each viewer's display and initial current month.

Contract tests must cover old request bodies, invalid IANA values, read-after-write, existing-profile backfill, and calendar month boundaries. ADR-026 and canonical OpenAPI are approved; implementation is pending.
