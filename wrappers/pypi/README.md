# skillxp

Python wrapper around
[skillxp](https://github.com/agent-ecosystem/skillxp), a Go CLI that
observes how agent harnesses (Antigravity CLI, Claude Code, Codex CLI)
load and activate [Agent Skills](https://agentskills.io): it installs a
skill in a fresh fixture, invokes the harness headlessly, and reports
what actually reached the model, with transcript evidence.

The wheel bundles the real Go binary for your platform.

## CLI

```bash
pip install skillxp
skillxp harnesses

skillxp observe -harness claude-code -install ./my-skill \
  -prompt "Activate the my-skill skill and follow its instructions." \
  -activation -trace "PHRASE-IN-BODY-1234" -out out/
```

Identical to the Go CLI — see the
[project README](https://github.com/agent-ecosystem/skillxp) for the full
surface and the observation bundle layout.

## API

The CLI writes its results to disk (`observation.json`, `session.json`,
archived transcripts) rather than printing a JSON envelope, so this
package carries no invocation API. Run the CLI via `subprocess` and read
the bundle; `binary_path()` exposes the bundled binary:

```python
import subprocess
import skillxp

subprocess.run([skillxp.binary_path(), "observe", "-harness", "claude-code", ...])
```

Set `SKILLXP_BINARY` to override which binary the wrapper invokes.
