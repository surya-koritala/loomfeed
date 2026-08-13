# Privacy Review Checklist

Use this checklist whenever a change adds or modifies authentication storage,
cookies, browser storage, telemetry, advertising, hosting/email processors, or
third-party embeds. It is a source-review checklist, not legal advice; each
operator remains responsible for counsel and compliance in the jurisdictions
where their instance operates.

## Trace the implementation

- [ ] Inventory every first- and third-party cookie: name, value/purpose,
  setter, domain/path, expiry, `HttpOnly`, `Secure`, `SameSite`, and the exact
  condition that creates it.
- [ ] Inventory credentials and preferences in `localStorage`,
  `sessionStorage`, IndexedDB, caches, service workers, and URLs.
- [ ] Trace new personal fields through request handling, database storage,
  logs, exports, deletion/anonymization, backups, and outbound processors.
- [ ] Check the current authentication sources, especially
  `internal/api/middleware/cookies.go`, `internal/api/handlers/oauth.go`,
  `web/src/lib/auth-hint.ts`, and `web/src/api/client.ts`.
- [ ] Check browser integrations and embeds in `web/src/app/layout.tsx`,
  `web/src/components/ClarityInit.tsx`, `web/src/components/EmbedRenderer.tsx`,
  and the relevant feature component.

## Review every processor

- [ ] Record the processor, purpose, data categories, recipients, retention,
  regions/transfers, and the operator configuration that enables it.
- [ ] Link to the processor's current official privacy/data documentation and
  verify the public description against it.
- [ ] Decide whether consent, an opt-out, restricted processing, or a regional
  configuration is required before any request leaves the browser.
- [ ] Treat user-triggered embeds and identity providers separately from
  scripts loaded on every page.
- [ ] Confirm bundled fonts and assets are actually self-hosted before saying
  they create no third-party request.

## Test disabled and enabled states

- [ ] Build/render with optional integration variables empty. Confirm no
  related script, request, cookie, local-storage entry, or beacon appears.
- [ ] Build/render with each variable enabled individually and together.
  Confirm the expected provider requests and storage, and no undisclosed ones.
- [ ] Visit `/privacy` in each configuration and confirm its enabled/disabled
  labels match the scripts the same build loads.
- [ ] Clear the browser profile between runs so old third-party storage cannot
  make a disabled build look enabled.
- [ ] Run the policy/config contract tests and the normal frontend lint, type
  check, unit tests, production build, and SSR health check.

## Update the public notice

- [ ] Describe new cookies/storage and optional processors before merging the
  implementation that introduces them.
- [ ] Remove claims that are no longer true and update the policy date.
- [ ] Keep deployment-specific statements out of the reusable source unless
  configuration can prove them.
- [ ] Self-hosters must supply their legal entity/controller, privacy contact,
  hosting and backup providers/regions, subprocessors, retention schedule,
  lawful bases, rights-request process, age rules, breach process, and required
  consent/opt-out controls.
- [ ] Ask for legal review when behavior or deployment jurisdiction changes;
  passing repository tests establishes source consistency, not legal
  sufficiency.
