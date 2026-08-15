# ADR-013: Canonical Five-Grade Recitation Scale

**Status:** Accepted  
**Date:** 2026-08-09  
**Deciders:** Karim (product owner)

## Context

Product and architecture documentation previously defined incompatible four- and
six-grade recitation scales. A single value set is required for API schemas,
PostgreSQL constraints, Go constants, Flutter labels, and progress analytics.

## Decision

Use exactly these stored values: `excellent`, `good`, `acceptable`,
`needs_review`, and `repeat`. Arabic and English labels remain presentation data;
the stored values are stable contract values. Future F-003/F-007 migrations must
introduce this constraint directly because the current implemented schema has no
recitation-grade columns.

## Consequences

- OpenAPI, database constraints, backend constants, and mobile mappings use one set.
- `very_good` is folded into `good`; `needs_improvement` is replaced by
  `needs_review`.
- Changing the stored values requires a new ADR and a backward-compatible data and
  API migration plan.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Four grades | Omits the pedagogically useful `acceptable` state. |
| Six grades | `very_good` adds teacher choice without a distinct workflow outcome. |
| Configurable values per circle | Breaks cross-circle analytics and adds premature schema complexity. |

## References

- [Constitution — frozen MVP business rules](../../../../.specify/memory/constitution.md#key-business-rules-frozen-for-mvp)
- [Architecture — domain enumerations](../ARCHITECTURE.md#40-domain-enumerations)
- [Feature F-003](../../../management/product/FEATURES.md#f-003-recitation-queue-system)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
