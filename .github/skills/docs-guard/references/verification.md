# Docs Guard — Verification Procedure

The mechanical heart of this skill: turn a document into a list of claims, then check each claim against the source of truth.

## Contents

- Step 1: Extract the claims
- Step 2: Verify each claim type
- Step 3: Record what you verified
- When you cannot verify

## Step 1: Extract the claims

Scan the doc and list every:

- Function, method, struct, constant, hook, event name
- CLI command, subcommand, flag, default value
- HTTP endpoint, method, status code, request/response field
- Config key, env var, file path, directory layout claim
- Version number, compatibility statement, dependency requirement
- Behavioral claim ("retries three times", "case-insensitive", "idempotent")

Inline code spans and code blocks are claim-dense; prose hides claims in verbs ("automatically reconnects" is a claim).

## Step 2: Verify each claim type

| Claim type | Source of truth | How |
|---|---|---|
| Symbol exists | The codebase | Grep definition — not usages, the definition |
| Go function signature | The definition site in `backend/` | Read parameters, defaults, return types; compare name-by-name with the doc |
| Dart class/method signature | The definition site in `mobile/lib/` | Read the actual declaration in the Dart file |
| OpenAPI endpoint | Route registration in Go router | Match path, method, and handler function |
| OpenAPI schema field | Go request/response struct | Match field names and types exactly |
| WebSocket event type | Go WebSocket event constants | Match the string value used in the handler |
| Config key / env var | The Go code that reads it (`os.Getenv`, config struct) | A documented key nothing reads is dead documentation |
| Default value | The definition, not the docs of the definition | Defaults drift silently; read the current line |
| Behavioral claim | The implementation path | Read the function; trace the claimed behavior |
| Internal link/anchor | The target file/heading | Resolve the relative path; slugify the heading and compare |

## Step 3: Record what you verified

In write-time mode, keep a short verification trail in your working notes (not in the doc): claim → file:line where confirmed. In review mode, this trail becomes your evidence — every finding cites the definition site that contradicts the doc.

When the runtime allows execution, prefer executable checks: run `go build ./...`, run `dart analyze`, run a link checker. When it does not, source-reading is the standard — never skip to "it looks right."

## When you cannot verify

If the source of truth is unavailable (private dependency, external service, missing schema):

1. Say so explicitly rather than guessing.
2. Downgrade the claim to what you can verify ("the client calls the `/v2/circles` endpoint" → verified in the Go router file, even if the server is unreachable).
3. Never decorate an unverified claim with confident language. "Should", "appears to", or a direct question to the user beats a fluent hallucination.
