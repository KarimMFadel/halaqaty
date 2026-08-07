# Docs Guard — Review Checklist

Structured walk for review mode. Findings first, file:line evidence always. Priorities: false claims → drift → substance → navigation.

## Contents

- Pass 1: Claim verification
- Pass 2: Code samples
- Pass 3: Drift scan
- Pass 4: Substance
- Pass 5: Navigation
- Halaqaty-specific pass
- Reporting

## Pass 1: Claim verification (must fix)

Run the full procedure in [verification.md](verification.md):

- Extract every symbol, flag, endpoint, config key, path, version, and behavioral claim.
- Verify each against its source of truth (definition site, parser, route table, changelog).
- Every unverified or contradicted claim is a Rule 1/3 finding with the contradicting file:line.
- Numbers and superlatives without a repo source are Rule 4 findings.

## Pass 2: Code samples (must fix)

For every fenced block, run the [code-samples.md](code-samples.md) checklist: imports, signatures, self-containment, local residue, secrets, shown output. Samples in docstrings count.

## Pass 3: Drift scan (should fix)

- Take the public API surface (or the diff, when reviewing a change) and grep the docs for renamed/removed symbols, old defaults, and dead flags.
- Check the changelog mentions what the docs claim is new, and versions agree (Rule 5).
- Cross-surface consistency: README vs reference vs docstrings vs config samples — one claim per fact, surfaces agreeing (Rule 6).

## Pass 4: Substance (should fix)

- Paraphrase docstrings, heading-restating sections, marketing adjectives, intro padding (Rule 7).
- Paraphrased upstream documentation that should be a link (Rule 8).
- Happy-path-only tutorials and API examples (Rule 9).
- Missing error responses (400/401/403/404/409/422) in OpenAPI endpoint specs.

## Pass 5: Navigation (worth noting)

- TOC vs actual headings; internal links and anchors resolve; no TODO stubs or "coming soon" in published docs (Rule 10).
- OpenAPI: all `$ref` values resolve, all `operationId` values are unique.

## Halaqaty-specific pass

After the standard passes, check:

1. **OpenAPI contract (`docs/contracts/openapi.yaml`)**: Does every new/changed endpoint appear here? Do all response schemas match the Go structs? Are all 6 error codes (400/401/403/404/409/422) present for new endpoints?
2. **WebSocket event catalog (`docs/contracts/ws_events.md`)**: Does every new/changed event type appear here? Does the payload schema match the Go event struct?
3. **ADRs (`docs/engineering/architecture/adr/`)**: Is the decision section real (not TBD)? Are alternatives listed? Is it sequentially numbered?
4. **Security documentation**: If a handler requires a `circle_members` role check, is that documented in the OpenAPI spec's security section or description?
5. **Spec artifacts (`specs/NNN-feature-name/`)**: Does the spec match what was implemented? Flag any drift.

## Reporting

Use the SKILL.md reporting format (Claim / Reality / Fix). Lead with the count: "N claims checked, M false, K unverifiable." End with a verdict — publish / fix first / do not publish — and at most three things the docs do well. A review that verifies 40 claims and finds 2 false is a *good* result; say so.
