# skillxp

Skill invocation runner: install an [Agent Skill](https://agentskills.io)
in a fresh fixture, invoke an agent harness headlessly, and report what
actually reached the model, with transcript evidence.

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

## Install

```bash
brew install agent-ecosystem/tap/skillxp
# or
npm install -g skillxp
# or
pip install skillxp
# or
go install github.com/agent-ecosystem/skillxp/cmd/skillxp@latest
```

As a Go library: `go get github.com/agent-ecosystem/skillxp`. The
harnesses you observe must be installed and authenticated.

## Quick start

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
One rule about phrases: never put a phrase you plan to trace in the
prompt, or it contaminates every echo location in the transcript.

As a library:

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

## Documentation

Full documentation is available at
**[skillxp.dev](https://skillxp.dev)**:

- [Quickstart](https://skillxp.dev/docs/quickstart/): install skillxp
  and observe your first skill invocation
- [Use Cases](https://skillxp.dev/docs/use-cases/): cross-platform skill
  CI, benchmarks, regression watching, and skill iteration
- [CLI](https://skillxp.dev/docs/cli/): the harnesses and observe
  commands, the observation bundle, and the trace report's
  classifications
- [Go Library](https://skillxp.dev/docs/library/): Observe, multi-turn
  sessions, repeat runs and rates, and the three packages
- [Sandboxing](https://skillxp.dev/docs/sandboxing/): isolated homes per
  run, user-scope installs, and per-harness auth setup
- [Harness Lore](https://skillxp.dev/docs/harness-lore/): the
  empirically established per-harness behavior the profiles encode

## License

MIT.
