---
title: Harness Lore
description: The per-harness skill behavior the profiles encode.
icon: menu_book
weight: 600
---

Everything below was established empirically, and it is the kind of
knowledge this tool exists to own once. Each `profile` cites the harness
version it was validated against.

## Discovery and activation

- Project-level skill directories: `.claude/skills` (claude-code),
  `.codex/skills` (codex), `.agents/skills` (antigravity).
- claude-code needs `--allowedTools Skill` for activation; antigravity
  needs permission bypass for its `view_file` pull; codex's read-only
  sandbox suffices.
- claude-code harness-pushes skill bodies into context (frontmatter
  stripped); codex and antigravity model-pull via file reads
  (frontmatter visible). Same spec, different vehicle, different
  author-facing result.

## Transcripts and evidence

- Antigravity transcripts record no injected context, so discovery
  evidence there is behavioral inference. One `agy -p` invocation also
  writes **two** conversations (a warm-up plus the real one), so
  attribution matches the recorded human prompt.
- Harnesses record symlink-resolved cwds; fixture paths are resolved
  before comparison (macOS `/var/folders` vs `/private/var/folders`).
- Echo locations that replay conversation content: codex
  `task_complete`, antigravity
  `user_input_context`/`conversation_history`/`checkpoint`. The trace
  report excludes them from loading evidence.

## Sandboxes and auth

- Sandbox redirection: codex honors `CODEX_HOME`; claude-code honors
  `CLAUDE_CONFIG_DIR` for config and transcripts but reads macOS
  credentials from the keychain (and migrates any `.credentials.json`
  into it), so sandboxed auth must come from `CLAUDE_CODE_OAUTH_TOKEN`
  or `ANTHROPIC_API_KEY`; antigravity keys everything off `HOME` and
  re-initiates browser OAuth if auth state doesn't transplant cleanly.
- Antigravity normally keeps its auth token in the macOS keychain (an
  item named "antigravity"), but falls back to a token file when no
  keychain is reachable, which is what makes HOME-swapped sandboxes
  possible at all.
- Antigravity ships builtin skills (`agy-customizations`,
  `permissioned-github`) under `antigravity-cli/builtin/skills/`:
  bundled baseline, like the built-ins on all three harnesses.
- Sandboxed fresh homes run with harness-default settings, including the
  default model, which may differ from the user's configured one;
  observations stamp the model per run for exactly this reason.

## Sessions

- Resume appends to the same transcript and session identity on all
  three harnesses (claude-code `--resume` keeps the preset session ID;
  codex and antigravity grow the same file), so multi-turn sessions stay
  addressable by the opening turn's ID.
