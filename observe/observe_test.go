package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/internal/harnesstest"
	"github.com/agent-ecosystem/skillxp/internal/invoker"
)

// stubHarness swaps the invocation seams for the test's lifetime.
func stubHarness(t *testing.T, cliVersion string, run func(context.Context, agentsummons.Request) (*agentsummons.Result, error)) {
	t.Helper()
	prevRun, prevVersion := invoker.Run, invoker.Version
	t.Cleanup(func() { invoker.Run, invoker.Version = prevRun, prevVersion })
	invoker.Version = func(ctx context.Context, id agentsummons.ID) (string, error) {
		return cliVersion, nil
	}
	invoker.Run = run
}

func TestObserveSessionNoTurns(t *testing.T) {
	_, err := ObserveSession(context.Background(), Config{}, agentsummons.ClaudeCode, SessionSpec{})
	if err == nil || !strings.Contains(err.Error(), "no turns") {
		t.Errorf("err = %v, want no-turns", err)
	}
}

func TestObserveSessionUnknownHarness(t *testing.T) {
	spec := SessionSpec{Turns: []Turn{{Prompt: "hi"}}}
	if _, err := ObserveSession(context.Background(), Config{}, agentsummons.ID("not-a-harness"), spec); err == nil {
		t.Error("want error for unknown harness")
	}
}

// A spec that cannot run must fail before the harness is touched at all —
// not even the version probe.
func TestUserSkillsRequireSandbox(t *testing.T) {
	var calls []agentsummons.Request
	stubHarness(t, "2.1.205", harnesstest.FakeClaudeCode(t, &calls))
	probes := 0
	prev := invoker.Version
	t.Cleanup(func() { invoker.Version = prev })
	invoker.Version = func(ctx context.Context, id agentsummons.ID) (string, error) {
		probes++
		return "2.1.205", nil
	}

	spec := SessionSpec{
		UserSkillDirs: []string{harnesstest.WriteSkill(t)},
		Turns:         []Turn{{Prompt: "hi"}},
	}
	_, err := ObserveSession(context.Background(), Config{}, agentsummons.ClaudeCode, spec)
	if err == nil || !strings.Contains(err.Error(), "Config.Sandbox") {
		t.Errorf("err = %v, want sandbox requirement", err)
	}
	if probes != 0 || len(calls) != 0 {
		t.Errorf("harness touched before validation failed: %d probes, %d invocations", probes, len(calls))
	}
}

// TestObserveSessionPipeline drives the full pipeline against a fake
// claude-code: fixture setup, project- and user-scope installs, sandbox
// wiring, locate, parse, resume, before-hooks, and archival.
func TestObserveSessionPipeline(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fake")
	var calls []agentsummons.Request
	fake := harnesstest.FakeClaudeCode(t, &calls)
	stubHarness(t, "2.1.205", func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		// The sandbox lives inside the fixture, which is torn down with
		// the run; the user-scope install must be checked at call time.
		configDir := harnesstest.ExtraEnv(req, "CLAUDE_CONFIG_DIR")
		if _, err := os.Stat(filepath.Join(configDir, "skills", "my-skill", "SKILL.md")); err != nil {
			t.Errorf("user-scope skill not in sandbox: %v", err)
		}
		return fake(ctx, req)
	})

	var beforeRan bool
	archive := filepath.Join(t.TempDir(), "archive")
	spec := SessionSpec{
		SkillDirs:     []string{harnesstest.WriteSkill(t)},
		UserSkillDirs: []string{harnesstest.WriteSkill(t)},
		Turns: []Turn{
			{Prompt: "activate the skill", Activation: true},
			{Prompt: "what did it say", Before: func(projectDir string) error {
				beforeRan = true
				if _, err := os.Stat(filepath.Join(projectDir, ".claude", "skills", "my-skill", "SKILL.md")); err != nil {
					t.Errorf("before-hook: project skill not installed: %v", err)
				}
				return nil
			}},
		},
	}
	cfg := Config{Sandbox: true, ArchiveDir: archive}
	so, err := ObserveSession(context.Background(), cfg, agentsummons.ClaudeCode, spec)
	if err != nil {
		t.Fatal(err)
	}

	if len(calls) != 2 {
		t.Fatalf("harness invoked %d times, want 2", len(calls))
	}
	opening, resume := calls[0], calls[1]
	if opening.SessionID == "" || opening.Resume != "" {
		t.Errorf("opening turn: SessionID=%q Resume=%q, want preset identity only", opening.SessionID, opening.Resume)
	}
	if len(opening.AllowedTools) != 1 || opening.AllowedTools[0] != "Skill" {
		t.Errorf("activation turn AllowedTools = %v, want [Skill]", opening.AllowedTools)
	}
	if resume.Resume != opening.SessionID || resume.SessionID != "" {
		t.Errorf("resume turn: SessionID=%q Resume=%q, want resume of %q", resume.SessionID, resume.Resume, opening.SessionID)
	}
	if len(resume.AllowedTools) != 0 {
		t.Errorf("passive resume turn AllowedTools = %v, want none", resume.AllowedTools)
	}
	if !beforeRan {
		t.Error("turn 2 before-hook never ran")
	}

	if so.SessionID != opening.SessionID || len(so.Turns) != 2 {
		t.Fatalf("session observation = %+v", so)
	}
	final := so.Final()
	if final != so.Turns[1] {
		t.Error("Final() is not the last turn")
	}
	if final.CLIVersion != "2.1.205" || final.HarnessVersion != "2.1.205" {
		t.Errorf("versions = %q / %q", final.CLIVersion, final.HarnessVersion)
	}
	if final.Model != "claude-fable-5" {
		t.Errorf("model = %q", final.Model)
	}
	if !final.Sandboxed || final.ExitCode != 0 || final.SessionID != opening.SessionID {
		t.Errorf("final observation = %+v", final)
	}

	// The final turn's parse covers the whole conversation.
	if len(final.Session.Events) < 4 {
		t.Errorf("final session has %d events, want both turns' records", len(final.Session.Events))
	}

	// Archived transcript survives the fixture teardown.
	if filepath.Dir(final.TranscriptPath) != archive {
		t.Errorf("final transcript %q not under archive %q", final.TranscriptPath, archive)
	}
	if _, err := os.Stat(final.TranscriptPath); err != nil {
		t.Errorf("archived transcript: %v", err)
	}
	// The first turn's path points into the (now removed) fixture store.
	if _, err := os.Stat(so.Turns[0].TranscriptPath); !os.IsNotExist(err) {
		t.Errorf("fixture transcript still on disk after teardown: %v", err)
	}
}

// agyUserInput renders an antigravity USER_INPUT step carrying the prompt.
func agyUserInput(t *testing.T, prompt string, ts time.Time) string {
	t.Helper()
	record, err := json.Marshal(map[string]any{
		"step_index": 0,
		"source":     "USER_EXPLICIT",
		"type":       "USER_INPUT",
		"status":     "DONE",
		"created_at": ts.UTC().Format(time.RFC3339),
		"content":    "<USER_REQUEST>\n" + prompt + "\n</USER_REQUEST>",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(record) + "\n"
}

// TestObserveSessionAntigravityFallbacks drives the pipeline against a fake
// antigravity: no preset session identity, attribution by time window, the
// session ID recovered from the conversation directory (ref.Meta), and the
// harness version stamped from the CLI version hint.
func TestObserveSessionAntigravityFallbacks(t *testing.T) {
	// Antigravity's sandbox clones a seed home; plant one under a fake HOME.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // windows
	seed := filepath.Join(home, ".skillxp", "seeds", "antigravity", "home", ".gemini")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "token"), []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}

	var calls []agentsummons.Request
	stubHarness(t, "1.1.9", func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		calls = append(calls, req)
		sandboxHome := harnesstest.ExtraEnv(req, "HOME")
		if sandboxHome == "" {
			t.Error("invoke: no HOME override in request env")
		}
		now := time.Now()
		path := filepath.Join(sandboxHome, ".gemini", "antigravity-cli", "brain",
			"conv-1", ".system_generated", "logs", "transcript_full.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(agyUserInput(t, req.Prompt, now)), 0o644); err != nil {
			return nil, err
		}
		return &agentsummons.Result{
			Harness:     req.Harness,
			Argv:        []string{"agy", req.Prompt},
			PromptIndex: 1,
			Workdir:     req.Workdir,
			Start:       now.Add(-time.Second),
			End:         now.Add(time.Second),
			ExitCode:    0,
		}, nil
	})

	spec := Spec{Prompt: "measure the skill", Activation: true}
	obs, err := Observe(context.Background(), Config{Sandbox: true}, agentsummons.Antigravity, spec)
	if err != nil {
		t.Fatal(err)
	}

	if len(calls) != 1 {
		t.Fatalf("harness invoked %d times, want 1", len(calls))
	}
	if !calls[0].AutoApprove {
		t.Error("antigravity activation turn must set AutoApprove")
	}
	if calls[0].SessionID != "" {
		t.Errorf("antigravity got preset SessionID %q; identity is discovered, never preset", calls[0].SessionID)
	}
	if obs.SessionID != "conv-1" {
		t.Errorf("session id = %q, want the conversation directory name conv-1", obs.SessionID)
	}
	if obs.HarnessVersion != "1.1.9" {
		t.Errorf("harness version = %q, want the CLI version hint", obs.HarnessVersion)
	}
	if len(obs.Session.Events) == 0 {
		t.Error("observation carries no parsed session")
	}
}

// A claude-code run that spawns subagents leaves agent-*.jsonl transcripts
// beside the session; archiving must copy them and repoint SubagentPaths,
// or the evidence vanishes with the fixture.
func TestArchiveIncludesSubagents(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fake")
	var calls []agentsummons.Request
	fake := harnesstest.FakeClaudeCode(t, &calls)
	stubHarness(t, "2.1.205", func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		res, err := fake(ctx, req)
		if err != nil {
			return nil, err
		}
		configDir := harnesstest.ExtraEnv(req, "CLAUDE_CONFIG_DIR")
		sub := filepath.Join(configDir, "projects", "-proj", req.SessionID, "subagents", "agent-sub1.jsonl")
		if err := os.MkdirAll(filepath.Dir(sub), 0o755); err != nil {
			return nil, err
		}
		record := harnesstest.SidechainUser(req.SessionID, "sub1", req.Workdir, "delegated subtask", "s-1", time.Now())
		if err := os.WriteFile(sub, []byte(record), 0o644); err != nil {
			return nil, err
		}
		return res, nil
	})

	archive := filepath.Join(t.TempDir(), "archive")
	cfg := Config{Sandbox: true, ArchiveDir: archive}
	obs, err := Observe(context.Background(), cfg, agentsummons.ClaudeCode, Spec{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}

	if len(obs.SubagentPaths) != 1 {
		t.Fatalf("subagent paths = %v, want one", obs.SubagentPaths)
	}
	sub := obs.SubagentPaths[0]
	if filepath.Dir(sub) != archive {
		t.Errorf("subagent transcript %q not repointed into archive %q", sub, archive)
	}
	if filepath.Base(sub) != "agent-sub1.jsonl" {
		t.Errorf("subagent transcript basename = %q", filepath.Base(sub))
	}
	if _, err := os.Stat(sub); err != nil {
		t.Errorf("archived subagent transcript: %v", err)
	}
}

func TestRepeatCountValidation(t *testing.T) {
	if _, err := Repeat(context.Background(), Config{}, agentsummons.ClaudeCode, SessionSpec{}, 0); err == nil {
		t.Error("want error for n=0")
	}
}

func TestRepeatRecordsFailuresAndContinues(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fake")
	var calls []agentsummons.Request
	fake := harnesstest.FakeClaudeCode(t, &calls)
	stubHarness(t, "2.1.205", func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		if len(calls) == 1 { // second invocation
			calls = append(calls, req)
			return nil, fmt.Errorf("harness fell over")
		}
		return fake(ctx, req)
	})

	archive := filepath.Join(t.TempDir(), "archive")
	spec := SessionSpec{Turns: []Turn{{Prompt: "hi"}}}
	cfg := Config{Sandbox: true, ArchiveDir: archive}
	ro, err := Repeat(context.Background(), cfg, agentsummons.ClaudeCode, spec, 3)
	if err != nil {
		t.Fatal(err)
	}
	if ro.Requested != 3 || len(ro.Runs) != 2 || len(ro.Errors) != 1 {
		t.Fatalf("repeat = requested %d, %d runs, %d errors", ro.Requested, len(ro.Runs), len(ro.Errors))
	}
	if ro.Errors[0].Run != 2 || !strings.Contains(ro.Errors[0].Err, "harness fell over") {
		t.Errorf("error = %+v, want run 2's failure", ro.Errors[0])
	}
	// Each successful run archives under its own run-NN subdirectory.
	for i, so := range ro.Runs {
		want := []string{"run-01", "run-03"}[i]
		if got := filepath.Base(filepath.Dir(so.Final().TranscriptPath)); got != want {
			t.Errorf("run %d archived under %q, want %q", i+1, got, want)
		}
	}
}

func TestRepeatAbortsOnCancellation(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fake")
	ctx, cancel := context.WithCancel(context.Background())
	invocations := 0
	stubHarness(t, "2.1.205", func(_ context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		invocations++
		cancel()
		return nil, fmt.Errorf("killed")
	})

	spec := SessionSpec{Turns: []Turn{{Prompt: "hi"}}}
	_, err := Repeat(ctx, Config{Sandbox: true}, agentsummons.ClaudeCode, spec, 5)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if invocations != 1 {
		t.Errorf("harness invoked %d times after cancellation, want 1", invocations)
	}
}

func TestTally(t *testing.T) {
	runs := []*SessionObservation{
		{SessionID: "a"}, {SessionID: "b"}, {SessionID: "a"},
	}
	got := Tally(runs, func(so *SessionObservation) string { return so.SessionID })
	if got["a"] != 2 || got["b"] != 1 || len(got) != 2 {
		t.Errorf("tally = %v", got)
	}
	if got := Tally(nil, func(*SessionObservation) string { return "x" }); len(got) != 0 {
		t.Errorf("empty tally = %v", got)
	}
}
