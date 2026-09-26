# Changelog

Notable changes to skillxp. Each version covers the Go module, the CLI,
and the npm/PyPI wrappers together (wrapper versions always match the Go
tag). Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `profile.TextForm`, the agentminutes text form each harness's
  transcript is parsed in, passed through `ParseOptions`. Copilot's is
  the delivered form: its `<skill-context>` wrapper is content the
  model acts on (the tags a content-wrapping check looks for, the
  skill's base directory, and a list of every file under the skill
  directory), and the bare form drops the tags.
- `profile.EnumeratesBundledFiles`, the lore that a harness lists a
  skill's bundled files in the context it delivers with the body
  (copilot 1.0.88). The drift probe now stages a bundled reference file
  beside every probe skill and reports drift when a harness that claims
  enumeration delivers the body without the file's name.
- The copilot profile's lore on reactivation: a repeat activation with
  the body unchanged is delivered again but logged by reference
  (`skill.invoked_ref`), which agentminutes resolves to the earlier
  body from its next release, so the second delivery traces as
  `harness-injected` too. Reported in issue #1.

### Changed

- Copilot observations trace the delivered text: a system event for a
  skill activation carries the `<skill-context>` tags and the file list
  around the body, where before it carried the body alone. Phrases
  traced from the body still surface; consumers matching the whole
  event text against the SKILL.md body see the wrapper now.

## [0.3.0] - 2026-09-25

### Added

- GitHub Copilot CLI (`copilot`) as the fourth harness, validated
  against 1.0.88. Project skills are staged at `.github/skills`
  (copilot also reads `.agents/skills` and `.claude/skills`) and user
  skills at `~/.copilot/skills`; both reached the listing from a
  sandbox. The discovery listing is the `<available_skills>` block of
  the system prompt, which the transcript records as a `system.message`
  on the session's opening turn (a resumed run records none). Activation needs no permission flag: a headless
  run executes read-only tools without a bypass and denies only writes,
  and the `skill` tool delivered the body with no flag at all. The
  session ID is preset (`--session-id`) and the transcript resolved by
  it directly, as on claude-code; resume preserves it. Every copilot
  run pins `COPILOT_AUTO_UPDATE=false`, since the CLI otherwise
  auto-updates on launch and could change release between the version
  probe and the run. Sandboxes set `COPILOT_HOME` (transcripts under
  `session-state/`, user skills under `skills/`), copy no auth material
  (the `/login` token lives in the macOS keychain, which the override
  does not hide), and clone `~/.skillxp/seeds/copilot` when present for
  hosts where copilot fell back to a plaintext `config.json`. The
  delivered skill body is recorded as model-visible text (the
  `skill.invoked` record, agentminutes v0.7.0+), so a phrase traced from
  the body surfaces as `harness-injected`, the evidence claude-code
  gives; the drift probe reports every copilot probe that way.

### Changed

- Bumped agentsummons to v0.4.0 and agentminutes to v0.7.0, which add
  Copilot CLI invocation and transcript parsing (agentminutes' locator
  honors `COPILOT_HOME`, the sandbox seam skillxp relies on). Their
  intermediate releases (v0.3.5, v0.5.2) revalidated their own axes
  against antigravity 1.2.11, claude-code 2.1.274, and codex 0.157.0.
  agentminutes v0.7.0 also gives a system event's `text` a stated
  contract (the record's model-visible text, bare), which widens what
  the trace report can see: Copilot's delivered skill body, Claude
  Code's injected attachments (system prompt, CLAUDE.md bodies,
  reminders, listings with their rendered headers), and Codex's system
  prompt (`session_meta/base_instructions`). Its schema `0.2.0` moves a
  Claude Code attachment's fields under `attachment.<key>` in
  `details`; skillxp reads no `details`, so bundles change only in what
  `session.json` carries.
- Revalidated the skill lore against those same releases
  (`profile.LastValidated`): the drift probe's project-scope, user-scope,
  and resume experiments all held on antigravity 1.2.11, claude-code
  2.1.274, and codex 0.157.0, so no profile changes were needed.

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
