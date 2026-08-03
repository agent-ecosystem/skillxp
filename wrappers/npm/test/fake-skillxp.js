#!/usr/bin/env node
"use strict";

// Fake skillxp binary for wrapper tests: behavior is driven by FAKE_* env
// vars so tests can assert the shim forwards stdio and exit codes
// untouched.
process.stdout.write(process.env.FAKE_STDOUT || "");
process.stderr.write(process.env.FAKE_STDERR || "");
process.exit(Number(process.env.FAKE_EXIT || 0));
