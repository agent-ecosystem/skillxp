---
title: Harness Lore
description: The per-harness skill behavior the profiles encode.
icon: menu_book
weight: 600
---

Everything below was established empirically, and it is the kind of
knowledge this tool exists to own once. Each `profile` field cites the
harness version it was established on, and `profile.LastValidated` (shown
by `skillxp harnesses`, compared by `skillxp doctor`) records the release
the whole profile was last re-confirmed on.

## Discovery and activation

- Project-level skill directories: `.claude/skills` (claude-code),
  `.codex/skills` (codex), `.agents/skills` (antigravity),
  `.github/skills` (copilot, which also reads `.agents/skills` and
  `.claude/skills`). User scope: `~/.copilot/skills` (copilot),
  `~/.gemini/config/skills` (antigravity), and the same `skills` folder
  under the config home for claude-code and codex.
- claude-code needs `--allowedTools Skill` for activation; antigravity
  needs permission bypass for its `view_file` pull; codex's read-only
  sandbox suffices; copilot needs no flag at all (headless runs execute
  read-only tools unprompted and deny only writes, and the `skill` tool
  is one of them).
- claude-code harness-pushes skill bodies into context (frontmatter
  stripped); codex and antigravity model-pull via file reads
  (frontmatter visible); copilot pushes through its `skill` tool, whose
  result is a one-line confirmation while the body is delivered as a
  `<skill-context>` block and recorded, frontmatter stripped, as the
  text of a `skill.invoked` record. Same spec, different vehicle,
  different author-facing result.
- Copilot's `<skill-context>` block is more than tags: its opening lines
  state the skill's base directory and list every file under the skill
  directory, recursively and unfiltered (nonstandard directories and
  files the body never mentions included), so the model learns of
  bundled files by name at activation. No other harness enumerates. The
  copilot profile parses in agentminutes' delivered text form so the
  tags and the list are in the traced text; the drift probe stages a
  bundled file and holds copilot to listing it.
- Copilot does not deduplicate reactivation. Activating a skill again
  with its body unchanged delivers the block again but logs it by
  reference (`skill.invoked_ref`: content hash, no body), which
  agentminutes resolves to the earlier body, so the second delivery
  traces as `harness-injected` like the first. An edited body logs a
  full `skill.invoked` instead.

## Transcripts and evidence

- Copilot records the system prompt on a session's opening turn (a
  resumed run records none), and its `<available_skills>` block is the
  discovery listing (`system.message`). The delivered skill body is the
  text of the `skill.invoked` record (or `skill.invoked_ref` on a repeat
  activation), byte-exact what the harness's own
  `skill.context_delivered_ref` hashes as delivered and wrapped in the
  `<skill-context>` block it records, so a phrase traced from the body
  is `harness-injected` evidence, as on claude-code, on every
  activation.
- Antigravity transcripts record no injected context, so discovery
  evidence there is behavioral inference. One `agy -p` invocation also
  writes **two** conversations (a warm-up plus the real one), so
  attribution matches the recorded human prompt.
- Harnesses record symlink-resolved cwds; fixture paths are resolved
  before comparison (macOS `/var/folders` vs `/private/var/folders`).
- Echo locations that replay conversation content: codex
  `task_complete`, antigravity
  `user_input_context`/`conversation_history`/`checkpoint`. The trace
  report excludes them from loading evidence. Copilot has none: its only
  system record with text is the system prompt itself.

## Sandboxes and auth

- Sandbox redirection: codex honors `CODEX_HOME`; claude-code honors
  `CLAUDE_CONFIG_DIR` for config and transcripts but reads macOS
  credentials from the keychain (and migrates any `.credentials.json`
  into it), so sandboxed auth must come from `CLAUDE_CODE_OAUTH_TOKEN`
  or `ANTHROPIC_API_KEY`; antigravity keys everything off `HOME` and
  re-initiates browser OAuth if auth state doesn't transplant cleanly;
  copilot honors `COPILOT_HOME` for config, skills, and transcripts
  while its `/login` token stays in the macOS keychain, so a fresh home
  is still authenticated.
- Copilot auto-updates on launch outside CI, so every copilot run pins
  `COPILOT_AUTO_UPDATE=false` to stay on the release the observation is
  attributed to.
- Antigravity normally keeps its auth token in the macOS keychain (an
  item named "antigravity"), but falls back to a token file when no
  keychain is reachable, which is what makes HOME-swapped sandboxes
  possible at all.
- Antigravity ships builtin skills (`agy-customizations`,
  `permissioned-github`) under `antigravity-cli/builtin/skills/`:
  bundled baseline, like the built-ins on every harness (copilot's are
  `customize-cloud-agent` and `github-pr-media`).
- Sandboxed fresh homes run with harness-default settings, including the
  default model, which may differ from the user's configured one;
  observations stamp the model per run for exactly this reason.

## Sessions

- Resume appends to the same transcript and session identity on every
  harness (claude-code and copilot `--resume` keep the preset session
  ID; codex and antigravity grow the same file), so multi-turn sessions
  stay addressable by the opening turn's ID.
