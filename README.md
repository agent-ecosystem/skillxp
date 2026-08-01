# skillxp

Skill invocation runner: install an [Agent
Skill](https://agentskills.io) in a fresh fixture, invoke an agent harness
headlessly, and report what actually reached the model, with transcript
evidence.

Skill authors publish for 25+ platforms that each load, present, and
manage skills differently, and mostly invisibly. skillxp makes that
behavior observable: it stages a skill on a real harness and answers
"what did the platform actually do with it" from the session transcript,
not from the model's self-reporting. It builds on
[agentsummons](https://github.com/agent-ecosystem/agentsummons) (headless
invocation) and
[agentminutes](https://github.com/agent-ecosystem/agentminutes)
(transcript parsing), and adds the third layer of lore: how each harness
discovers, activates, and records skills.

skillxp renders no verdicts. It produces observations; graders consume
them. The first consumer is the
[agent-skill-implementation](https://github.com/agent-ecosystem/agent-skill-implementation)
loading benchmark, whose checks and verdict logic live in that repo's
`benchmark-runner/`.

## Status

Early development. Supported harnesses (validated versions): Antigravity
CLI 1.1.4, Claude Code 2.1.205, Codex CLI 0.144.6. Results reflect
**headless** behavior, which may differ from interactive use.

## CLI

```bash
# Where does each harness discover project-level skills?
skillxp harnesses

# Stage a skill, activate it, and trace how two phrases
# reached the model
skillxp observe -harness claude-code -install ./my-skill \
  -prompt "Activate the my-skill skill and follow its instructions." \
  -activation -trace "PHRASE-IN-BODY-1234,PHRASE-IN-REFERENCE-5678" \
  -out out/
```

`observe` writes a bundle: `observation.json` (run metadata and the
trace report), `session.json` (the normalized transcript), and the
archived native transcript(s) that evidence line numbers point into.
With `-runs N`, each repetition gets a fresh fixture and session under
`run-NN/`, and `summary.json` reports how many runs delivered each traced
phrase to each location — the rate report for model-dependent behavior
(one run is an anecdote; N runs are a rate).

The trace report classifies each occurrence:

- `harness-injected` — the platform put it in front of the model
  (harness-push loading)
- `tool-result` — the model fetched it with a tool (model-pull loading)
- `model-output` — the model emitted it
- `human-prompt` / `echo` — your own prompt, or a system record replaying
  conversation content; never loading evidence

Never put a phrase you plan to trace in the prompt: a literal-minded model
truthfully answers "it's in your message", and the phrase contaminates
every echo location in the transcript.

## Library

```go
obs, err := observe.Observe(ctx, observe.Config{ArchiveDir: dir},
    agentsummons.ClaudeCode, observe.Spec{
        SkillDirs:  []string{"./my-skill"},
        Prompt:     "Activate the my-skill skill and follow its instructions.",
        Activation: true,
    })
if err != nil {
    return err
}
occs := trace.Phrase(obs.Session, "PHRASE-IN-BODY-1234", obs.Profile.EchoSubtypes)
```

Multi-turn experiments run several prompts against one harness session
(every supported harness appends resumed turns to the same transcript),
with optional between-turn hooks for editing skill files on disk:

```go
so, err := observe.ObserveSession(ctx, cfg, agentsummons.ClaudeCode, observe.SessionSpec{
    SkillDirs: []string{"./my-skill"},
    Turns: []observe.Turn{
        {Prompt: "Activate the my-skill skill.", Activation: true},
        {Prompt: "Activate the my-skill skill again.", Activation: true,
            Before: func(projectDir string) error { return editSkill(projectDir) }},
    },
})
```

Each turn yields an Observation whose Session covers the whole
conversation so far; `so.Final().Session` is the complete record.

For model-dependent behavior, `observe.Repeat` runs the same session spec
N times (serialized, fresh fixture each) and records failed runs as data
rather than aborting; `observe.Tally` counts runs by whatever label your
classifier assigns:

```go
ro, err := observe.Repeat(ctx, cfg, agentsummons.ClaudeCode, sessSpec, 10)
rates := observe.Tally(ro.Runs, func(so *observe.SessionObservation) string {
    if len(trace.Phrase(so.Final().Session, phrase, echo)) > 0 {
        return "activated"
    }
    return "not-activated"
})
```

Packages:

- `profile` — per-harness skill lore: install paths, headless activation
  permissions, transcript attribution, evidence-vs-echo locations. Each
  profile cites the harness version it was validated against.
- `observe` — fixture → invoke → locate → parse → archive. Returns an
  `Observation`; runs must be serialized (transcript attribution and
  context isolation both require it).
- `trace` — content provenance over normalized sessions: given any phrase,
  how did it reach (or leave) the model? Only
  model-visible text counts; raw provenance and harness sidecar data are
  excluded (grepping those produces false positives). Also: discovery
  listing detection, skill-path tool-read detection.

## Uses

- **Cross-platform skill CI**: does your skill activate, and do its
  reference files actually load, on every harness you ship to? The
  dynamic complement to
  [skill-validator](https://github.com/agent-ecosystem/skill-validator)'s
  static checks.
- **Benchmarks**: the agent-skill-implementation loading benchmark drives
  its checks through skillxp.
- **Harness regression watching**: rerun on each harness release and diff
  observations; loading behavior changes silently between versions.
- **Skill iteration**: A/B two phrasings of a description, N runs each,
  compare activation behavior with measurements instead of vibes.

## Sandboxing

`-sandbox` (CLI) or `Config.Sandbox` (library) runs the harness against an
isolated home cloned per run, so user-level state can neither leak into
the experiment (your own installed skills appearing in listings) nor be
polluted by it. Bundled skills still appear: harnesses ship built-ins
(claude-code's own skill set, codex's imagegen) that exist in every home,
sandboxed or not — treat them as harness baseline, not contamination. Sandboxing also unlocks user-scope installs
(`-install-user` / `SessionSpec.UserSkillDirs`), which are refused
unsandboxed: skillxp never touches the real user scope.

Auth is the per-harness wrinkle; each needs at most one one-time setup
step, and every failure mode is a guided error:

| Harness | Isolation | One-time setup |
|---|---|---|
| codex | `CODEX_HOME` | None: `auth.json` (+`config.toml`) is copied from `~/.codex`, or from `~/.skillxp/seeds/codex/` if you prefer a curated seed |
| claude-code | `CLAUDE_CONFIG_DIR` | `claude setup-token`, exported as `CLAUDE_CODE_OAUTH_TOKEN` (bills to your subscription; macOS keychain credentials are unreachable from a sandboxed config dir). To keep the token off disk, store it in 1Password, export the `op://` reference instead, and run under `op run -- <command>`; an unresolved reference is a guided error. |
| antigravity | `HOME` | Authenticate once into a persistent seed: `mkdir -p ~/.skillxp/seeds/antigravity/home && HOME=~/.skillxp/seeds/antigravity/home agy`, log in, quit; runs clone the seed. macOS will report a missing keychain during login — click **Cancel**: agy then stores its token as a file inside the seed (`antigravity-oauth-token`), which is exactly what HOME-swapped clones can use. Never click "Reset To Defaults". |

Two safety properties are deliberate: sandboxed claude-code runs empty
`ANTHROPIC_API_KEY` so a stray key in your environment cannot outrank the
subscription token and silently switch to metered API billing, and
sandboxed antigravity runs set `BROWSER=false` so a failed auth
transplant errors in text instead of opening a login page in your
browser.

## Harness lore the profiles encode

Established empirically; the kind of thing this tool exists to own once:

- Project-level skill directories: `.claude/skills` (claude-code),
  `.codex/skills` (codex), `.agents/skills` (antigravity).
- claude-code needs `--allowedTools Skill` for activation; antigravity
  needs permission bypass for its `view_file` pull; codex's read-only
  sandbox suffices.
- claude-code harness-pushes skill bodies into context (frontmatter
  stripped); codex and antigravity model-pull via file reads (frontmatter
  visible). Same spec, different vehicle, different author-facing result.
- Antigravity transcripts record no injected context, so discovery
  evidence there is behavioral inference; and one `agy -p` invocation
  writes **two** conversations (a warm-up plus the real one), so
  attribution matches the recorded human prompt.
- Harnesses record symlink-resolved cwds; fixture paths are resolved
  before comparison (macOS `/var/folders` vs `/private/var/folders`).
- Echo locations that replay conversation content: codex `task_complete`,
  antigravity `user_input_context`/`conversation_history`/`checkpoint`.
- Antigravity normally keeps its auth token in the macOS keychain (an
  item named "antigravity"), but falls back to a token file when no
  keychain is reachable — which is what makes HOME-swapped sandboxes
  possible at all. Antigravity also ships builtin skills
  (`agy-customizations`, `permissioned-github`) under
  `antigravity-cli/builtin/skills/`: bundled baseline on all three
  harnesses.
- Sandboxed fresh homes run with harness-default settings — including
  the default model, which may differ from the user's configured one;
  observations stamp the model per run for exactly this reason.
- Sandbox redirection: codex honors `CODEX_HOME`; claude-code honors
  `CLAUDE_CONFIG_DIR` for config and transcripts but reads macOS
  credentials from the keychain (and migrates any `.credentials.json`
  into it), so sandboxed auth must come from `CLAUDE_CODE_OAUTH_TOKEN` or
  `ANTHROPIC_API_KEY`; antigravity keys everything off `HOME` and
  re-initiates browser OAuth if auth state doesn't transplant cleanly.
- Resume appends to the same transcript and session identity on all three
  harnesses (claude-code `--resume` keeps the preset session ID; codex and
  antigravity grow the same file), so multi-turn sessions stay addressable
  by the opening turn's ID.

## Roadmap

- npm and PyPI wrapper packages, matching agentsummons/agentminutes.

## License

MIT.
