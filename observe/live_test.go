package observe

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/profile"
	"github.com/agent-ecosystem/skillxp/trace"
)

// TestLiveSmoke runs one real skill-activation observation against an
// installed, authenticated harness. It bills real tokens and depends on
// machine state, so it is opt-in and never runs in CI:
//
//	SKILLXP_E2E=claude-code go test ./observe/ -run TestLiveSmoke -v
//
// claude-code additionally needs CLAUDE_CODE_OAUTH_TOKEN (see
// profile.PrepareSandbox); other harnesses need their sandbox seeds.
func TestLiveSmoke(t *testing.T) {
	harnessID := os.Getenv("SKILLXP_E2E")
	if harnessID == "" {
		t.Skip("set SKILLXP_E2E=<harness-id> to run the live smoke test")
	}
	p, err := profile.For(agentsummons.ID(harnessID))
	if err != nil {
		t.Fatal(err)
	}

	// A minimal skill whose body carries a canary phrase no model would
	// produce unprompted.
	const canary = "aurora-quench-73"
	skill := filepath.Join(t.TempDir(), "smoke-skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: smoke-skill\ndescription: Reports the current codeword when activated.\n---\n\nThe codeword is " + canary + ". State it plainly.\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Sandbox: true,
		Log:     t.Logf,
	}
	spec := Spec{
		SkillDirs:  []string{skill},
		Prompt:     p.ActivationPrompt("smoke-skill"),
		Activation: true,
	}
	obs, err := Observe(context.Background(), cfg, p.Harness, spec)
	if err != nil {
		t.Fatal(err)
	}

	if obs.ExitCode != 0 {
		t.Errorf("harness exit code = %d", obs.ExitCode)
	}
	if obs.SessionID == "" || len(obs.Session.Events) == 0 {
		t.Errorf("observation lacks a session: id=%q events=%d", obs.SessionID, len(obs.Session.Events))
	}
	occs := trace.Phrase(obs.Session, canary, p.EchoSubtypes)
	if len(occs) == 0 {
		t.Errorf("canary never surfaced; skill likely did not load (model=%s version=%s)", obs.Model, obs.HarnessVersion)
	}
	for _, o := range occs {
		t.Logf("canary at %s (event %d, %s)", o.Location, o.EventIndex, o.Kind)
	}
}
