# Changelog

Notable changes to skillxp. Each version covers the Go module, the CLI,
and the npm/PyPI wrappers together (wrapper versions always match the Go
tag). Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
