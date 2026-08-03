# skillxp

Node wrapper around
[skillxp](https://github.com/agent-ecosystem/skillxp), a Go CLI that
observes how agent harnesses (Antigravity CLI, Claude Code, Codex CLI)
load and activate [Agent Skills](https://agentskills.io): it installs a
skill in a fresh fixture, invokes the harness headlessly, and reports
what actually reached the model, with transcript evidence.

Installing this package delivers the real Go binary for your platform via
an optional dependency (no install scripts).

Windows is temporarily unavailable on npm (a registry naming issue is
being worked out with npm support); use the
[PyPI package](https://pypi.org/project/skillxp/) or a
[release binary](https://github.com/agent-ecosystem/skillxp/releases)
with `SKILLXP_BINARY` in the meantime.

## CLI

```bash
npx skillxp harnesses

npx skillxp observe -harness claude-code -install ./my-skill \
  -prompt "Activate the my-skill skill and follow its instructions." \
  -activation -trace "PHRASE-IN-BODY-1234" -out out/
```

Identical to the Go CLI — see the
[project README](https://github.com/agent-ecosystem/skillxp) for the full
surface and the observation bundle layout.

## API

The CLI writes its results to disk (`observation.json`, `session.json`,
archived transcripts) rather than printing a JSON envelope, so this
package carries no invocation API. Spawn the CLI yourself and read the
bundle; `binaryPath()` exposes the bundled binary:

```js
const { binaryPath } = require("skillxp");
const { execFileSync } = require("node:child_process");

execFileSync(binaryPath(), ["observe", "-harness", "claude-code", ...]);
```

Set `SKILLXP_BINARY` to override which binary the wrapper invokes.
