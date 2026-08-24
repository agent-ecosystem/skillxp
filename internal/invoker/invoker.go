// Package invoker holds the agentsummons call seams. Production code calls
// the harness through these vars so tests — observe's own and the CLI's —
// can swap in a fake harness without the public API growing test hooks.
package invoker

import "github.com/agent-ecosystem/agentsummons"

var (
	// Run invokes a harness headlessly; see agentsummons.Run.
	Run = agentsummons.Run

	// Version probes an installed harness's version; see
	// agentsummons.Version.
	Version = agentsummons.Version
)
