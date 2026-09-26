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
# The full lore check is `skillxp drift probe` (see "The drift devtool").
SKILLXP_E2E=claude-code go test ./observe/ -run TestLiveSmoke -v
```

## Testing conventions

Hermetic tests never invoke a real harness. Production code calls
agentsummons through the `internal/invoker` seam, and tests swap it for
`internal/harnesstest.FakeClaudeCode` (or `FakeCopilotWith`), which
writes genuine transcript records into the sandbox store so the real
locate and parse pipeline runs against real files. Extend those helpers rather than spawning
harnesses; the only test that talks to a real harness is the opt-in
live smoke test above. Locate-attribution tests build synthetic
transcript stores per harness layout (see `profile/locate_test.go`)
and always pass explicit roots so tests never touch `~/.claude`,
`~/.codex`, `~/.copilot`, or `~/.gemini`. CI enforces an 80% statement-coverage
floor; the JSON field names of observation bundles are pinned by
contract tests (`observe/contract_test.go`) because graders parse
them.

## What this repo is (and is not)

skillxp owns skill-loading *observation* knowledge: where each harness
discovers skills, how activation is requested, and what the transcript
records about injected context. Invocation mechanics live in
[agentsummons](https://github.com/agent-ecosystem/agentsummons);
transcript discovery and parsing live in
[agentminutes](https://github.com/agent-ecosystem/agentminutes). skillxp
imports both and renders no verdicts: it produces observation bundles,
and graders (benchmark runners, skill CI) consume them.

## Harness lore versioning

Every harness profile in `profile/` is empirical lore: where skills are
discovered at project and user scope, which system-event subtypes carry
the discovery listing and which merely echo the conversation, what
permissions an activation turn needs, how the run's transcript is
attributed, and that resumed turns append to the opening turn's
transcript. All of it can move with a harness release without warning, so it is
versioned the way the sibling libraries version their surfaces:
`profile.LastValidated` records the newest release each profile was
re-confirmed on. It is the third validation axis next to
`agentsummons.LastValidated` (the flag surface) and agentminutes'
`harness.LastValidated` (the transcript format); the three drift
independently and the tables are deliberately separate. Like them it is
coverage documentation, not a compatibility bound: newer releases usually
keep working, and a moved skill directory fails loudly (the skill never
loads) rather than going unnoticed.

Per-field comments in `profile.go` still cite the release a fact was
*established* on (e.g. antigravity's user scope moving in 1.1.9); the
table records the release the whole profile was last *re-confirmed* on.
`TestLastValidatedCoversProfiles` keeps the table and the profile list in
step. `skillxp harnesses` prints both axes this build knows about.

## The drift devtool

The shape, vocabulary, and exit codes mirror agentminutes' drift devtool
so the three libraries reconcile drift the same way.

- `skillxp doctor` — free, user-facing: installed version per harness
  against `profile.LastValidated`, like `agentsummons doctor`. A newer
  installed release is a drift candidate, not a failure. Exit 0 clean
  (missing harnesses are fine), 1 drift candidate, 2 a version probe
  failed. Run it before spending tokens on anything.
- `skillxp drift probe [-harness id,...] [-force] [-keep] [-timeout d]`
  — maintainer-only (implemented in `internal/driftprobe/`; absent from
  usage; spends tokens). Gates on installed version vs `LastValidated`
  (equal → skip unless `-force`; older → skip), then runs three fixed
  probes per harness, every one sandboxed (`Config.Sandbox`) with a fresh
  canary-bearing skill and the profile's activation prompt:
  `project` (project-scope discovery), `user` (user-scope discovery), and
  `resume` (a passive opening turn, then activation on the resumed
  session). Each probe grades the lore in two tiers. **Drift** is the
  harness contradicting the profile: the skill absent from the listing
  subtypes the profile names (claude-code `attachment/skill_listing`,
  codex `message/developer`, copilot `system.message`), the transcript not locatable or parseable
  where the profile says (`observe.TranscriptError`), or a resumed turn
  missing from the opening turn's session. **Inconclusive** is the
  model's doing: the skill was listed but its body never surfaced in a
  loading location, so the probe retries once with an insistent prompt
  and only then gives up. Antigravity records no listing, so there a
  missing body is inconclusive after the retry and only run failures and
  attribution are drift. Green always means the lore was exercised. Exit
  codes: 0 clean, 1 drift, 2 inconclusive, 3 execution error (harness
  missing when named explicitly, sandbox not seeded, nonzero exit).

Per-harness prerequisites are the sandbox ones (`profile.PrepareSandbox`,
[Sandboxing](https://skillxp.dev/docs/sandboxing/)): claude-code needs
`CLAUDE_CODE_OAUTH_TOKEN` (mint with `claude setup-token`; keeping it in a
secret manager and resolving inline, e.g.
`CLAUDE_CODE_OAUTH_TOKEN="$(op item get <item> --fields password --reveal)"`,
keeps it off disk); codex needs `~/.codex/auth.json` or a seed under
`~/.skillxp/seeds/codex`; antigravity needs the one-time seed home under
`~/.skillxp/seeds/antigravity/home`; copilot needs nothing on macOS (its
keychain token stays reachable under `COPILOT_HOME`) and, on a host where
it stored the token in a plaintext `config.json`, that file under
`~/.skillxp/seeds/copilot/` or a `COPILOT_GITHUB_TOKEN`. A forced run on
every harness at its validated version looks like this and takes a few
minutes:

```
$ skillxp drift probe -force
antigravity:
  probing 1.2.11 (last validated 1.2.11; --force)
  probe project: discovered and loaded; body reached the model via tool-result, model-output
  probe user: discovered and loaded; body reached the model via tool-result, model-output
  probe resume: discovered and loaded; body reached the model via tool-result, model-output
  clean: antigravity at 1.2.11 still matches its profile
claude-code:
  probing 2.1.274 (last validated 2.1.274; --force)
  probe project: discovered and loaded; body reached the model via harness-injected, model-output
  ...
```

Reconciling drift: re-establish the moved fact in `profile.Profiles()`
(a skill directory, a listing or echo subtype, a locate strategy), citing
the release in the field comment as the existing ones do; update
`profile.LastValidated`; extend the hermetic tests (the fake harness in
`internal/harnesstest/` models discovery and listing, so a moved location
gets a fixture there); and describe the change under Unreleased in
`CHANGELOG.md`. A clean probe on a newer release needs only the table
bump and a changelog line. Keep `-keep` transcripts around while doing
this; they are the evidence.

## Releasing

Distribution matches agentsummons: goreleaser builds the archives and
pushes the Homebrew formula to
[agent-ecosystem/homebrew-tap](https://github.com/agent-ecosystem/homebrew-tap);
the npm and PyPI wrappers repackage those same archives in the release
workflow, so wrapper versions can never drift from the Go tag.

Cutting a release:

1. agentsummons and agentminutes release first; bump both requirements
   in `go.mod` to their fresh tags and re-run the hermetic suite. Then
   run `skillxp doctor`, and `skillxp drift probe` against any harness
   `profile.LastValidated` trails (or `-force` to re-confirm the lot),
   and reconcile per "The drift devtool" above. The hermetic suite cannot
   see lore drift; only the probe can.
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
