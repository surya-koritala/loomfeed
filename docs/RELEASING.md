# Releasing Loomfeed

Loomfeed repository releases use Semantic Versioning tags in the form
`vMAJOR.MINOR.PATCH`. Product milestones organize work, but a milestone name is
not itself proof that a software release exists. A release exists only when its
annotated tag and GitHub Release have been published.

The web application and official SDK packages retain their own package
versions. A repository release does not publish npm or Python packages unless a
separate package-release change explicitly adds and tests that workflow.

## Release requirements

- Release only a commit reachable from `main`.
- Require the normal protected-branch checks to pass on that exact commit.
- Use an annotated `vMAJOR.MINOR.PATCH` tag; never move or reuse a release tag.
- Add a dated `CHANGELOG.md` section before creating the tag.
- Create the GitHub Release as a draft, verify its notes, tag target, and assets,
  then publish it. Repository release immutability locks the published tag and
  assets and generates a release attestation. See GitHub's
  [immutable release documentation](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases).
- Mark only the newest stable release as `Latest`. Use a prerelease suffix and
  GitHub's prerelease flag for preview builds.

## Maintainer procedure

1. Open a focused release-preparation pull request that moves relevant entries
   out of `Unreleased`, adds the release date, and updates comparison links.
2. Merge only after all required checks pass. Record the resulting `main` SHA
   and verify its push CI run also succeeds.
3. Create and push an annotated tag:

   ```bash
   git fetch origin main --tags
   git tag -a vX.Y.Z <main-sha> -m "Loomfeed vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. The `Prepare Release` workflow validates that the tag is annotated, appears
   in the changelog, and points into `main`, then creates a draft GitHub Release
   using the matching changelog section.
5. Check the draft's commit, notes, and any attached assets. Publish it only
   after those checks are correct.
6. Verify the published source archives are available, the release is
   immutable, and its tag resolves to the recorded commit. Do not delete and
   recreate a published version.

Historical releases created before the workflow must follow the same checks
manually. Do not create several version tags on one commit merely to mirror old
planning milestones; that would imply release boundaries that never existed.

## Hotfixes and previews

- A compatible production fix increments `PATCH`.
- A backward-compatible feature release increments `MINOR`.
- A breaking public contract increments `MAJOR`.
- Preview releases use standard SemVer suffixes such as `v2.0.0-rc.1` and must
  be marked as prereleases. The automated stable-tag workflow intentionally
  accepts only three-part stable versions; preview releases are prepared and
  published manually until a tested preview workflow is added.
