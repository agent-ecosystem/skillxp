# Development

## Commands

```bash
go test ./... -count=1        # hermetic tests (CI also runs windows-latest)
golangci-lint run             # lint + gofumpt (CI-enforced)
GOOS=windows go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...  # release artifacts are static

# Wrapper tests
npm test --prefix wrappers/npm
python3 -m unittest discover -s wrappers/pypi/tests
```

## What this repo is (and is not)

skillxp owns skill-loading *observation* knowledge: where each harness
discovers skills, how activation is requested, and what the transcript
records about injected context. Invocation mechanics live in
[agentsummons](https://github.com/agent-ecosystem/agentsummons);
transcript discovery and parsing live in
[agentminutes](https://github.com/agent-ecosystem/agentminutes). skillxp
imports both and renders no verdicts: it produces observation bundles,
and graders (benchmark runners, skill CI) consume them.

## Releasing

Distribution matches agentsummons: goreleaser builds the archives and
pushes the Homebrew formula to
[agent-ecosystem/homebrew-tap](https://github.com/agent-ecosystem/homebrew-tap);
the npm and PyPI wrappers repackage those same archives in the release
workflow, so wrapper versions can never drift from the Go tag.

Cutting a release:

1. Bump the agentsummons/agentminutes requirements in `go.mod` if their
   validated flag/format surfaces moved, and re-run the suite.
2. Promote the Unreleased section of `CHANGELOG.md` to the new version
   heading. The release workflow extracts the tag's section for the
   GitHub release notes and **fails the release if the section is
   missing**, so this step cannot be skipped.
3. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`. The Release
   workflow does the rest: GitHub release + archives, brew formula
   (`Formula/skillxp.rb` in the tap), npm packages (six platform
   packages + the `skillxp` main package), and PyPI platform wheels.

### One-time setup before the first release

- **`HOMEBREW_TAP_TOKEN` repo secret**: a token with push access to
  agent-ecosystem/homebrew-tap (same token agentsummons uses); goreleaser
  pushes the formula with it.
- **PyPI**: no manual publish needed. Add a *pending* trusted publisher
  on pypi.org (Account settings → Publishing → "Create a new pending
  publisher") for project `skillxp`, repo `agent-ecosystem/skillxp`,
  workflow `release.yml`. The pending publisher becomes the project's
  normal publisher on the workflow's first upload.
- **npm**: trusted publishing can only be configured on a package that
  already exists, so the first publish of each package is manual, with a
  logged-in npm session (Node ≥ 24 for a current npm):

  ```bash
  # after the v0.1.0 GitHub release exists
  gh release download v0.1.0 --dir assets --pattern '*.tar.gz' --pattern '*.zip'
  node wrappers/npm/scripts/build-packages.mjs --version 0.1.0 --assets assets --out dist-npm
  for pkg in ./dist-npm/platform/*/; do npm publish "$pkg" --access public; done
  npm publish ./dist-npm/skillxp --access public
  ```

  Then on npmjs.com, for **each of the seven packages** (`skillxp` and
  `skillxp-{darwin,linux}-{arm64,x64}`, `skillxp-win32-{arm64,x64}`):
  Settings → Trusted publishing → GitHub Actions, repo
  `agent-ecosystem/skillxp`, workflow `release.yml`. From the next tag
  the workflow publishes tokenlessly; its already-published check makes
  re-running a partially failed publish safe.

  For the first release, the workflow's `publish-npm` job will fail
  (no trusted publisher yet) — that's expected; do the manual publish
  above and re-run the job to confirm the skip-if-published logic, or
  just leave it for v0.1.1.

The npm/PyPI wrappers are distribution-only (CLI passthrough plus
`binaryPath()`/`binary_path()`): the CLI writes observation bundles to
disk rather than printing a JSON envelope, so there is no `run`-style
API to mirror, unlike agentsummons. If an envelope-emitting command ever
lands, mirror agentsummons' wrapper API shape.
