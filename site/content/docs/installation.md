---
title: Installation
description: Homebrew, npm, PyPI, go install, and prebuilt binaries.
icon: download
weight: 200
---

## CLI

```sh
brew install agent-ecosystem/tap/skillxp
```

```sh
npm install -g skillxp
```

```sh
pip install skillxp
```

```sh
go install github.com/agent-ecosystem/skillxp/cmd/skillxp@latest
```

The npm and pip packages wrap the same prebuilt Go binary; nothing extra
is downloaded at install time. Prebuilt static binaries are also on the
[releases page](https://github.com/agent-ecosystem/skillxp/releases).

On Windows, use pip or a release binary: the npm package temporarily has
no Windows support (a registry naming issue is being worked out with
npm).

Verify any install with:

```sh
skillxp version
```

## Library

```sh
go get github.com/agent-ecosystem/skillxp
```

See [Go Library](/docs/library/) for the API.

## Harnesses

skillxp invokes real harnesses, so the harnesses you observe must be
installed and authenticated: `claude` (Claude Code), `codex` (Codex
CLI), and `agy` (Antigravity CLI). Sandboxed runs have one extra
one-time auth step per harness; see
[Sandboxing](/docs/sandboxing/#per-harness-auth).
