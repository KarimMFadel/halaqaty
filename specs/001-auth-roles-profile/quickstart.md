# Quickstart: 001-auth-roles-profile

1. Treat `docs/contracts/openapi.yaml` as the canonical contract. Keep the feature contract synchronized; run `make api-lint` (or `spectral lint docs/contracts/openapi.yaml`) before coding.
2. Apply the next additive migration after `000010`; verify an upgrade from a database containing `000010` and a fresh-schema migration. Do not edit an already-applied migration.
3. Implement Firebase bearer verification first, then require a matching `X-Halaqaty-Session-ID` on every protected endpoint except `POST /auth/register` and `POST /auth/sessions`.
4. Implement circle creation and role changes as database transactions with locked membership rows; emit audit events and preserve the final-teacher invariant.
5. In Flutter, use Firebase SDK sign-up/sign-in and secure storage for the backend session ID; obtain a fresh Firebase ID token for each protected request and clear local session state on `401`.
6. Run Go unit, integration, and contract tests; Flutter widget and integration tests; then `make api-lint`, Go formatting/lint, Flutter analyze/format, and secret scanning.
