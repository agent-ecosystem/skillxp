# Changelog

Notable changes to skillxp. Each version covers the Go module, the CLI,
and the npm/PyPI wrappers together (wrapper versions always match the Go
tag). Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.2.0] - 2026-09-20

### Added

- `profile.LastValidated`: the release each harness profile's skill lore
  was last re-confirmed on, the third validation axis next to
  agentsummons' flag surface and agentminutes' transcript format. A
  registry test keeps it in step with the profile list.
- `skillxp doctor`: installed harness versions against
  `profile.LastValidated`, mirroring `agentsummons doctor` (free; exit 1
  on a drift candidate, 2 on a failed version probe).
- `skillxp drift probe` (maintainer-only, spends tokens): re-runs the
  sandboxed project-scope, user-scope, and resume skill-loading
  experiments against the installed release and grades the profile's
  lore, with the version gate, one-shot retry, drift-vs-inconclusive
  split, and exit codes (0/1/2/3) of agentminutes' drift devtool. See
  DEVELOPMENT.md.
- `observe.TranscriptError`: a typed error for a turn whose transcript
  could not be located or parsed as the profile expects, distinct from a
  failed invocation.

### Changed

- `skillxp harnesses` now prints both validation axes per harness
  (`validated`, the skill lore; `flags`, the agentsummons flag surface).
- Bumped agentsummons to v0.3.4 and agentminutes to v0.5.1: both
  revalidated against antigravity 1.2.7 / claude-code 2.1.267 / codex
  0.155.1 (no flag or format surfaces moved for skillxp; agentminutes
  now parses Claude Code `cost-state` records as `system` events, and
  agentsummons documents antigravity 1.2.6's unlimited `--print-timeout`
  default and `AGY_ERROR` exit 3). Skill discovery at project and user
  scope, skill-listing evidence, and resume attribution were re-confirmed
  live on those same harness versions; no profile changes were needed.
- `skillxp harnesses` now prints the harness version each release was
  validated against (from agentsummons' `LastValidated` table), so the
  numbers update automatically with dependency bumps. The README points
  there instead of hardcoding versions that went stale.

## [0.1.3] - 2026-09-13

### Changed

- `observe.ObserveSession` now rejects a spec with user-scope skill
  installs but no `Config.Sandbox` before probing the harness, so the
  misconfiguration surfaces as the sandbox-requirement error even when
  the harness is not installed (previously the not-usable error won).
- Bumped agentsummons to v0.3.3 and agentminutes to v0.5.0:
  agentsummons revalidated its flag surface against antigravity 1.2.2 /
  claude-code 2.1.236 / codex 0.154.0 (with new antigravity headless
  caveats around `denied_actions`, the stderr `error:` marker, and
  `--print-timeout` truncation), and agentminutes made subagent
  sessions first-class for all three harnesses (codex multi-agent
  rollouts parse fully with `is_subagent`/`subagent_id` metadata).

## [0.1.2] - 2026-08-24

### Changed

- Bumped agentsummons to v0.3.2 and agentminutes to v0.4.0: both
  revalidated against agy 1.1.19 / claude-code 2.1.231 / codex 0.149.1,
  with agentminutes absorbing codex 0.149.1's item_completed transcript
  format and adding the derived `total_prompt_tokens` to session
  totals.

## [0.1.1] - 2026-08-03

### Fixed

- npm: dropped the `skillxp-win32-arm64` and `skillxp-win32-x64`
  platform packages — npm's registry spam detection blocked creating
  both names on the first publish, and `skillxp@0.1.0` shipped
  optionalDependencies pointing at packages that don't exist (harmless
  on macOS/Linux, but no binary and a misleading error on Windows).
  Windows npm users get an actionable pointer to the PyPI package or a
  release binary with `SKILLXP_BINARY`; the entries return when npm
  support frees the names. GitHub release archives and PyPI wheels
  still cover Windows on both architectures.

## [0.1.0] - 2026-08-03

Initial release.

### Added

- `skillxp harnesses`: show supported harnesses (Antigravity CLI,
  Claude Code, Codex CLI) and their skill install locations.
- `skillxp observe`: install skill(s) in a fresh fixture, invoke a
  harness headlessly, and write an observation bundle — run metadata and
  trace report (`observation.json`), the normalized transcript
  (`session.json`), and the archived native transcript(s). Supports
  phrase tracing (`-trace`), repeated runs with per-run fixtures and a
  cross-run summary (`-runs`), skill-activation permission requests
  (`-activation`), user-scope installs against an isolated home
  (`-install-user` with `-sandbox`), and fixture retention (`-keep`).
- Distribution: Homebrew formula (`agent-ecosystem/tap/skillxp`), npm
  package (`skillxp`), PyPI package (`skillxp`), and prebuilt archives
  on GitHub releases for darwin/linux/windows on amd64/arm64.
