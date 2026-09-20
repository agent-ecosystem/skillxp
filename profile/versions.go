package profile

import "github.com/agent-ecosystem/agentsummons"

// LastValidated records the newest release of each harness whose skill
// lore in Profiles was re-confirmed live: skill discovery at project and
// user scope, the discovery-listing and echo subtypes, the activation
// permissions, transcript attribution, and resume behavior. It is the
// third validation axis next to agentsummons.LastValidated (the flag
// surface) and agentminutes' harness.LastValidated (the transcript
// format); the three drift independently, so the tables are deliberately
// separate. Like them it is coverage documentation, not a compatibility
// bound: newer releases usually keep working, and `skillxp drift probe`
// is how a newer release earns its entry (see DEVELOPMENT.md).
// Alphabetical. Callers must treat the map as read-only.
var LastValidated = map[agentsummons.ID]string{
	agentsummons.Antigravity: "1.2.7",
	agentsummons.ClaudeCode:  "2.1.267",
	agentsummons.Codex:       "0.155.1",
}
