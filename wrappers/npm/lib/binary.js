"use strict";

// Node (platform, arch) pairs with a published platform package; mirrors
// PLATFORMS in scripts/build-packages.mjs. Alphabetical.
const SUPPORTED = new Set([
  "darwin-arm64",
  "darwin-x64",
  "linux-arm64",
  "linux-x64",
  "win32-arm64",
  "win32-x64",
]);

// binaryPath resolves the skillxp binary: the SKILLXP_BINARY override
// first, then the platform package installed via optionalDependencies.
function binaryPath() {
  const override = process.env.SKILLXP_BINARY;
  if (override) return override;
  const key = `${process.platform}-${process.arch}`;
  if (!SUPPORTED.has(key)) {
    throw new Error(
      `skillxp: no prebuilt binary for ${key}; install the Go CLI instead ` +
        "(https://github.com/agent-ecosystem/skillxp) and set SKILLXP_BINARY",
    );
  }
  const exe = process.platform === "win32" ? "skillxp.exe" : "skillxp";
  try {
    return require.resolve(`skillxp-${key}/bin/${exe}`);
  } catch {
    throw new Error(
      `skillxp: platform package skillxp-${key} is missing; it installs ` +
        "automatically as an optional dependency, so reinstall with optional " +
        "dependencies enabled, or set SKILLXP_BINARY to a binary you provide",
    );
  }
}

module.exports = { binaryPath };
