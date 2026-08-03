#!/usr/bin/env python3
"""Fake skillxp binary for wrapper tests: behavior is driven by FAKE_*
env vars so tests can point SKILLXP_BINARY at a controllable stand-in."""

import os
import sys

sys.stdout.write(os.environ.get("FAKE_STDOUT", ""))
sys.stderr.write(os.environ.get("FAKE_STDERR", ""))
sys.exit(int(os.environ.get("FAKE_EXIT", "0")))
