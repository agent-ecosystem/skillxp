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

# Live smoke test: one real skill-activation run against an installed,
# authenticated harness. Opt-in, bills real tokens, never runs in CI.
SKILLXP_E2E=claude-code go test ./observe/ -run TestLiveSmoke -v
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
3. Bump the hero badge version in `site/data/landing.yaml`
   (`hero.badge.text`) and republish the docs site.
4. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`. The Release
   workflow does the rest: GitHub release + archives, brew formula
   (`Formula/skillxp.rb` in the tap), npm packages (platform packages +
   the `skillxp` main package), and PyPI platform wheels.

npm currently ships four platform packages (darwin/linux × arm64/x64):
npm's spam detection blocked creating both `skillxp-win32-*` names on
the v0.1.0 publish. When npm support frees them, restore the win32
entries in `wrappers/npm/scripts/build-packages.mjs` (PLATFORMS),
`wrappers/npm/lib/binary.js` (SUPPORTED), and
`wrappers/npm/package.json` (optionalDependencies), and drop the
Windows caveat from `wrappers/npm/README.md`. Windows users are covered
by PyPI wheels and release archives meanwhile.

### One-time setup (done for v0.1.0, recorded for posterity)

- **`HOMEBREW_TAP_TOKEN` repo secret**: a token with push access to
  agent-ecosystem/homebrew-tap (same token agentsummons uses); goreleaser
  pushes the formula with it.
- **PyPI**: no manual publish was needed. A *pending* trusted publisher
  on pypi.org (project `skillxp`, repo `agent-ecosystem/skillxp`,
  workflow `release.yml`) converted to the project's normal publisher on
  the workflow's first upload.
- **npm**: trusted publishing can only be configured on a package that
  already exists, so the five v0.1.0 packages were published manually
  (`npm publish --access public --otp=...` on the build-packages.mjs
  output). Each package then needs, on npmjs.com: Settings → Trusted
  publishing → GitHub Actions, repo `agent-ecosystem/skillxp`, workflow
  `release.yml`. From the next tag the workflow publishes tokenlessly;
  its already-published check makes re-running a partially failed
  publish safe.

The npm/PyPI wrappers are distribution-only (CLI passthrough plus
`binaryPath()`/`binary_path()`): the CLI writes observation bundles to
disk rather than printing a JSON envelope, so there is no `run`-style
API to mirror, unlike agentsummons. If an envelope-emitting command ever
lands, mirror agentsummons' wrapper API shape.
