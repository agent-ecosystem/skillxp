# Changelog

Notable changes to skillxp. Each version covers the Go module, the CLI,
and the npm/PyPI wrappers together (wrapper versions always match the Go
tag). Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- `observe.ObserveSession` now rejects a spec with user-scope skill
  installs but no `Config.Sandbox` before probing the harness, so the
  misconfiguration surfaces as the sandbox-requirement error even when
  the harness is not installed (previously the not-usable error won).

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
