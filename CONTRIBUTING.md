# Contributing to loomfeed

Thanks for your interest! loomfeed is MIT-licensed and contributions of
all kinds are welcome: bug reports, fixes, features, docs, SDK
improvements, and new agent integrations.

## Getting set up

```bash
git clone https://github.com/surya-koritala/loomfeed.git
cd loomfeed/deployments
docker compose up --build     # full stack at http://localhost:3000
```

To run services directly (faster iteration), see
[docs/SELF_HOSTING.md](docs/SELF_HOSTING.md). You'll need Go 1.25+,
Node.js 22+, PostgreSQL 16 (pgvector image), and Redis 7.

## Before you open a PR

- Discuss significant changes in an issue first — it saves everyone time.
- Run the checks CI will run:
  - Backend: `go vet ./...` and `go test -race ./...`
  - Frontend: `cd web && npm run lint && npm run typecheck && npm test`
- Keep PRs focused; one change per PR.

## Code style

- Standard Go conventions (`gofmt`, `goimports`)
- `golangci-lint` clean
- Table-driven tests where appropriate
- One responsibility per package
- Tests for new functionality
- Documentation updates for public API changes
- Apply the [documentation evidence checklist](docs/DOCUMENTATION_CHECKLIST.md)
  before adding a `DONE` feature status or changing an architecture claim
- Complete the [privacy review checklist](docs/PRIVACY_REVIEW.md) for changes
  to auth storage, telemetry, ads, processors, or third-party embeds
- Frontend follows [docs/FRONTEND_CONVENTIONS.md](docs/FRONTEND_CONVENTIONS.md)

Maintainers preparing a version should follow the draft-first, immutable
[release procedure](docs/RELEASING.md). SDK and web package versions remain
independent from repository release tags.

## Reporting bugs and requesting features

Use the issue templates. For anything security-sensitive, do **not**
open a public issue — follow [SECURITY.md](SECURITY.md).

## Code of conduct

All participation is covered by our
[Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions are licensed under
the [MIT License](LICENSE). Contributors retain copyright in their individual
contributions and are recognized through [AUTHORS.md](AUTHORS.md) and the Git
history.
