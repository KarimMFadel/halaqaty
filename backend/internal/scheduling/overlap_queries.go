package scheduling

// ponytail: one lock serializes overlap-sensitive writes globally; replace it
// with interval-scoped locks if scheduling write throughput becomes material.
const lockOverlapChecksQuery = `SELECT pg_advisory_xact_lock(6, 51)`
