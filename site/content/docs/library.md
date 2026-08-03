---
title: Go Library
description: Observe, multi-turn sessions, repeat runs, and the three packages.
icon: code
weight: 400
---

## Observe

To run one observation from Go, use `observe.Observe`:

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

## Multi-turn sessions

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

## Repeat runs and rates

For model-dependent behavior, `observe.Repeat` runs the same session
spec N times (serialized, fresh fixture each) and records failed runs as
data rather than aborting; `observe.Tally` counts runs by whatever label
your classifier assigns:

```go
ro, err := observe.Repeat(ctx, cfg, agentsummons.ClaudeCode, sessSpec, 10)
rates := observe.Tally(ro.Runs, func(so *observe.SessionObservation) string {
    if len(trace.Phrase(so.Final().Session, phrase, echo)) > 0 {
        return "activated"
    }
    return "not-activated"
})
```

## The packages

- `profile`: per-harness skill lore, covering install paths, headless
  activation permissions, transcript attribution, and evidence-vs-echo
  locations. Each profile cites the harness version it was validated
  against.
- `observe`: fixture, invoke, locate, parse, archive. Returns an
  `Observation`; runs must be serialized, because transcript attribution
  and context isolation both require it.
- `trace`: content provenance over normalized sessions. Given any
  phrase, how did it reach (or leave) the model? Only model-visible text
  counts; raw provenance and harness sidecar data are excluded, because
  grepping those produces false positives. Also covers discovery listing
  detection and skill-path tool-read detection.
