---
title: CLI
description: The harnesses, doctor, and observe commands, the observation bundle, and the trace report.
icon: terminal
weight: 300
---

## skillxp harnesses

To see where each supported harness discovers project-level skills, and
which harness releases this build was validated against, use
`harnesses`:

```sh
$ skillxp harnesses
antigravity    validated 1.2.11    flags 1.2.11    project skills: .agents/skills   does NOT record injected context (evidence is inference)
claude-code    validated 2.1.274   flags 2.1.274   project skills: .claude/skills   records injected context
codex          validated 0.157.0   flags 0.157.0   project skills: .codex/skills    records injected context
copilot        validated 1.0.88    flags 1.0.88    project skills: .github/skills   records injected context
```

`validated` is the newest release each harness's skill lore (discovery
locations, listing evidence, resume behavior) was re-confirmed on;
`flags` is the release the underlying
[agentsummons](https://agentsummons.dev) flag surface was validated on.
Both record coverage, not a compatibility bound: newer releases usually
keep working.

The injected-context column matters for reading results: on a harness
that records injected context, discovery evidence is direct; on
antigravity, it is behavioral inference. See
[Harness Lore](/docs/harness-lore/).

## skillxp doctor

To compare the harness versions you have installed against the validated
ones, use `doctor`. It is free: nothing runs beyond each harness's
version command.

```sh
$ skillxp doctor
antigravity  installed 1.2.11, validated 1.2.11 — clean
claude-code  installed 2.1.274, validated 2.1.274 — clean
codex        installed 0.158.0 > validated 0.157.0 — drift candidate; run `skillxp drift probe` to revalidate
copilot      installed 1.0.88, validated 1.0.88 — clean
```

A drift candidate is a statement about validation coverage, not a
failure: observations still run, and a moved skill directory fails
loudly (the skill never loads) rather than going unnoticed. The exit code makes
`doctor` usable as a gate: 0 clean (harnesses that are not installed do
not count), 1 at least one drift candidate, 2 a version probe failed.

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
