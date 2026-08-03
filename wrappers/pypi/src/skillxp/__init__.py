"""Python wrapper around the skillxp Go CLI, which observes how agent
harnesses (Antigravity CLI, Claude Code, Codex CLI) load and activate
Agent Skills.

The wheel bundles the real binary; the ``skillxp`` console script is a
transparent passthrough. The CLI writes its results to disk
(``observation.json``, ``session.json``, archived transcripts) rather
than printing a JSON envelope, so this package carries no invocation
API: run the CLI via ``subprocess`` with ``binary_path()`` and read the
bundle. Set SKILLXP_BINARY to override which binary is invoked.
"""

import os
import subprocess
import sys


class NotInstalledError(RuntimeError):
    """No bundled binary for this platform (and no SKILLXP_BINARY override)."""


def binary_path():
    """Return the path of the skillxp binary this wrapper invokes."""
    override = os.environ.get("SKILLXP_BINARY")
    if override:
        return override
    exe = "skillxp.exe" if sys.platform == "win32" else "skillxp"
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "bin", exe)
    if not os.path.exists(path):
        raise NotInstalledError(
            "skillxp: no bundled binary for this platform; install the Go "
            "CLI instead (https://github.com/agent-ecosystem/skillxp) and "
            "set SKILLXP_BINARY"
        )
    # Some installers drop the exec bit on package data; restore best-effort.
    if os.name == "posix" and not os.access(path, os.X_OK):
        try:
            os.chmod(path, 0o755)
        except OSError:
            pass
    return path


def _main():
    """Console-script entry: transparent passthrough to the binary."""
    try:
        binary = binary_path()
    except NotInstalledError as err:
        print(err, file=sys.stderr)
        raise SystemExit(69)  # EX_UNAVAILABLE: no usable binary
    argv = [binary] + sys.argv[1:]
    if os.name == "posix":
        os.execv(binary, argv)
    raise SystemExit(subprocess.call(argv))
