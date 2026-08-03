---
title: Sandboxing
description: Isolated homes per run, user-scope installs, and per-harness auth.
icon: security
weight: 500
---

`-sandbox` (CLI) or `Config.Sandbox` (library) runs the harness against
an isolated home cloned per run, so user-level state can neither leak
into the experiment (your own installed skills appearing in listings)
nor be polluted by it.

Bundled skills still appear: harnesses ship built-ins (claude-code's own
skill set, codex's imagegen) that exist in every home, sandboxed or not.
Treat them as harness baseline, never as contamination.

Sandboxing also unlocks user-scope installs (`-install-user` /
`SessionSpec.UserSkillDirs`), which are refused unsandboxed: skillxp
never touches the real user scope.

## Per-harness auth

Auth is the per-harness wrinkle; each needs at most one one-time setup
step, and every failure mode is a guided error:

| Harness | Isolation | One-time setup |
|---|---|---|
| codex | `CODEX_HOME` | None: `auth.json` (+`config.toml`) is copied from `~/.codex`, or from `~/.skillxp/seeds/codex/` if you prefer a curated seed |
| claude-code | `CLAUDE_CONFIG_DIR` | `claude setup-token`, exported as `CLAUDE_CODE_OAUTH_TOKEN` (bills to your subscription; macOS keychain credentials are unreachable from a sandboxed config dir). To keep the token off disk, store it in 1Password, export the `op://` reference instead, and run under `op run -- <command>`; an unresolved reference is a guided error. |
| antigravity | `HOME` | Authenticate once into a persistent seed: `mkdir -p ~/.skillxp/seeds/antigravity/home && HOME=~/.skillxp/seeds/antigravity/home agy`, log in, quit; runs clone the seed. macOS will report a missing keychain during login. Click **Cancel**: agy then stores its token as a file inside the seed (`antigravity-oauth-token`), which is exactly what HOME-swapped clones can use. Never click "Reset To Defaults". |

## Safety properties

Two safety properties are deliberate:

- Sandboxed claude-code runs empty `ANTHROPIC_API_KEY`, so a stray key
  in your environment cannot outrank the subscription token and switch
  to metered API billing without you noticing.
- Sandboxed antigravity runs set `BROWSER=false`, so a failed auth
  transplant errors in text instead of opening a login page in your
  browser.
