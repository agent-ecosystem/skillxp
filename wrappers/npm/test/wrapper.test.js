"use strict";

const assert = require("node:assert/strict");
const { execFile } = require("node:child_process");
const path = require("node:path");
const { test } = require("node:test");

const { binaryPath } = require("../lib/index");

const FAKE = path.join(__dirname, "fake-skillxp.js");
const SHIM = path.join(__dirname, "..", "bin", "skillxp.js");

test("binaryPath honors SKILLXP_BINARY", () => {
  const saved = process.env.SKILLXP_BINARY;
  process.env.SKILLXP_BINARY = FAKE;
  try {
    assert.equal(binaryPath(), FAKE);
  } finally {
    if (saved === undefined) delete process.env.SKILLXP_BINARY;
    else process.env.SKILLXP_BINARY = saved;
  }
});

test("binaryPath without override reports the missing platform package", () => {
  const saved = process.env.SKILLXP_BINARY;
  delete process.env.SKILLXP_BINARY;
  try {
    // The repo checkout has no platform packages installed, so resolution
    // must fail with the actionable message (never a bare module error).
    assert.throws(() => binaryPath(), /skillxp/);
  } finally {
    if (saved !== undefined) process.env.SKILLXP_BINARY = saved;
  }
});

test("shim passes stdio through and propagates the exit code", () => {
  return new Promise((resolve, reject) => {
    execFile(
      process.execPath,
      [SHIM, "observe", "-harness", "claude-code"],
      {
        env: {
          ...process.env,
          SKILLXP_BINARY: FAKE,
          FAKE_STDOUT: "out",
          FAKE_STDERR: "err",
          FAKE_EXIT: "3",
        },
      },
      (err, stdout, stderr) => {
        try {
          assert.equal(err && err.code, 3);
          assert.equal(stdout, "out");
          assert.equal(stderr, "err");
          resolve();
        } catch (assertion) {
          reject(assertion);
        }
      },
    );
  });
});

test("shim exits 69 with the actionable message when no binary resolves", () => {
  return new Promise((resolve, reject) => {
    const env = { ...process.env };
    delete env.SKILLXP_BINARY;
    execFile(process.execPath, [SHIM, "harnesses"], { env }, (err, _stdout, stderr) => {
      try {
        assert.equal(err && err.code, 69);
        assert.match(stderr, /skillxp/);
        resolve();
      } catch (assertion) {
        reject(assertion);
      }
    });
  });
});
