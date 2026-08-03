---
title: skillxp
description: "Skill invocation runner: stage an Agent Skill on a real harness and report what actually reached the model, with transcript evidence."
---

skillxp is a skill invocation runner: install an
[Agent Skill](https://agentskills.io) in a fresh fixture, invoke an agent
harness headlessly, and report what actually reached the model, with
transcript evidence.

Skill authors publish for 25+ platforms that each load, present, and
manage skills differently, and mostly invisibly. skillxp makes that
behavior observable: it stages a skill on a real harness and answers
"what did the platform actually do with it" from the session transcript,
never from the model's self-reporting.

It builds on [agentsummons](https://agentsummons.dev) (headless
invocation) and [agentminutes](https://agentminutes.dev) (transcript
parsing), and adds the third layer of lore: how each harness discovers,
activates, and records skills.
