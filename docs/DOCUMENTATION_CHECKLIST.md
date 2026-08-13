# Documentation Evidence Checklist

Use this checklist before marking a feature `DONE` or making an architectural
claim in `README.md`, `ROADMAP.md`, `docs/FEATURE_STATUS.md`, or
`docs/ARCHITECTURE.md`.

## `DONE` feature claims

- Link the current implementation that provides the behavior. Planned,
  scaffolded, seeded, or unreachable code is not implementation evidence.
- Link every public route or protocol entry point used to reach it.
- Link the current migration or schema definition when persistence, indexes, or
  database extensions are part of the claim.
- Link a reachable UI entry point whenever the wording promises a page,
  dashboard, browser, marketplace, wizard, or other visual surface.
- Link a representative test that exercises the advertised behavior. A model
  or fixture test alone does not prove a route or UI claim.
- Run the linked test and exercise the route or UI before changing the status.
  If any promised layer is absent, use `PARTIAL` or `NOT BUILT` and name the
  missing layer.

## Architecture claims

- Name the algorithm, extension, datastore, and delivery guarantee that the
  running code actually uses. Do not substitute a planned or adjacent
  technology (for example, BM25 for PostgreSQL `ts_rank_cd`).
- Trace storage claims through both the active migration chain and the runtime
  repository query. A standalone SQL seed file is not migration evidence.
- Distinguish optional configuration from the default deployment and identify
  the enabling setting when relevant.

## Cross-document review

- Search all four public claim surfaces for the old and new terminology:
  `README.md`, `ROADMAP.md`, `docs/FEATURE_STATUS.md`, and
  `docs/ARCHITECTURE.md`.
- Update or remove duplicate claims in the same change.
- Use repository-relative links so evidence remains reviewable in forks and
  offline checkouts.
