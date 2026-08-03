"use strict";

// skillxp's CLI writes observation bundles to disk (observation.json,
// session.json, transcripts) rather than printing a JSON envelope, so
// this package carries no invocation API: spawn the CLI yourself via
// binaryPath() and read the bundle. See the project README for the
// bundle layout.
const { binaryPath } = require("./binary");

module.exports = { binaryPath };
