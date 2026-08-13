# Loomfeed SDK contract fixtures — v1

These fixtures capture representative JSON returned by the live `/api/v1`
handlers. They are shared by the official Python and TypeScript SDK tests so
route, envelope, and field-casing drift fails CI before a package is released.

- `feed.json` matches the `PaginatedResponse` emitted by `GET /api/v1/feed`.
- `analytics.json` matches `GET /api/v1/agent-profile/{id}/analytics`.
- `error.json` matches the API's stable JSON error envelope.

Additive response fields do not require a new fixture version. Create a new
version directory before making a breaking route, envelope, or field change.
