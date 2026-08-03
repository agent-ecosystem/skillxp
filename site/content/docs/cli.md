---
title: CLI
description: The harnesses and observe commands, the observation bundle, and the trace report.
icon: terminal
weight: 300
---

## skillxp harnesses

To see where each supported harness discovers project-level skills, use
`harnesses`:

```sh
$ skillxp harnesses
antigravity    project skills: .agents/skills   does NOT record injected context (evidence is inference)
claude-code    project skills: .claude/skills   records injected context
codex          project skills: .codex/skills    records injected context
```

The injected-context column matters for reading results: on a harness
that records injected context, discovery evidence is direct; on
antigravity, it is behavioral inference. See
[Harness Lore](/docs/harness-lore/).

## skillxp observe

To stage a skill, activate it, and trace how phrases reached the model,
use `observe`:

```sh
skillxp observe -harness claude-code -install ./my-skill \
  -prompt "Activate the my-skill skill and follow its instructions." \
  -activation -trace "PHRASE-IN-BODY-1234,PHRASE-IN-REFERENCE-5678" \
  -out out/
```

`observe` writes a bundle:

- `observation.json`: run metadata and the trace report
- `session.json`: the normalized transcript (an
  [agentminutes](https://agentminutes.dev) session record)
- the archived native transcript(s) that evidence line numbers point
  into

With `-runs N`, each repetition gets a fresh fixture and session under
`run-NN/`, and `summary.json` reports how many runs delivered each
traced phrase to each location. This is the rate report for
model-dependent behavior: one run is an anecdote; N runs are a rate.

## The trace report

The trace report classifies each occurrence of a traced phrase:

- `harness-injected`: the platform put it in front of the model
  (harness-push loading)
- `tool-result`: the model fetched it with a tool (model-pull loading)
- `model-output`: the model emitted it
- `human-prompt` / `echo`: your own prompt, or a system record replaying
  conversation content; never loading evidence

## Choosing phrases to trace

Never put a phrase you plan to trace in the prompt: a literal-minded
model truthfully answers "it's in your message", and the phrase
contaminates every echo location in the transcript. Seed the phrases in
the skill body and reference files instead, and let the trace report
show whether they arrived.

## Sandboxing

`-sandbox` runs the harness against an isolated home cloned per run,
and unlocks user-scope installs (`-install-user`), which are refused
unsandboxed. See [Sandboxing](/docs/sandboxing/) for the isolation
model and per-harness auth setup.
