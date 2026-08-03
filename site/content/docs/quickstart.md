---
title: Quickstart
description: Install skillxp and observe your first skill invocation.
icon: rocket_launch
weight: 100
---

## Install

macOS users can `brew` install:

```sh
brew install agent-ecosystem/tap/skillxp
```

For other install options, including npm, PyPI, go install, and prebuilt
binaries, refer to [Installation](/docs/installation/).

## See what each harness does with skills

To see where each supported harness discovers project-level skills, and
whether its transcripts record injected context, use the `harnesses`
command:

```sh
$ skillxp harnesses
antigravity    project skills: .agents/skills   does NOT record injected context (evidence is inference)
claude-code    project skills: .claude/skills   records injected context
codex          project skills: .codex/skills    records injected context
```

## Observe a skill invocation

To stage a skill in a fresh fixture, activate it, and trace how two
phrases reached the model, use `observe`:

```sh
skillxp observe -harness claude-code -install ./my-skill \
  -prompt "Activate the my-skill skill and follow its instructions." \
  -activation -trace "PHRASE-IN-BODY-1234,PHRASE-IN-REFERENCE-5678" \
  -out out/
```

The bundle written to `out/` contains `observation.json` (run metadata
and the trace report), `session.json` (the normalized transcript), and
the archived native transcript(s) that evidence line numbers point into.
See [CLI](/docs/cli/) for the trace report's classifications and the one
rule about choosing phrases.
