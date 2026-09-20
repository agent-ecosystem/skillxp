package driftprobe

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/internal/harnesstest"
	"github.com/agent-ecosystem/skillxp/internal/invoker"
	"github.com/agent-ecosystem/skillxp/profile"
)

// newer is a claude-code version past LastValidated, so the gate probes.
const newer = "99.0.0"

// stub swaps the invocation seams for the test's lifetime and supplies
// the sandbox credential claude-code's profile insists on.
func stub(t *testing.T, version string, versionErr error, b harnesstest.Behavior) *[]agentsummons.Request {
	t.Helper()
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fake")
	prevRun, prevVersion := invoker.Run, invoker.Version
	t.Cleanup(func() { invoker.Run, invoker.Version = prevRun, prevVersion })
	invoker.Version = func(ctx context.Context, id agentsummons.ID) (string, error) {
		return version, versionErr
	}
	var calls []agentsummons.Request
	invoker.Run = harnesstest.FakeClaudeCodeWith(t, &calls, b)
	return &calls
}

func runClaude(t *testing.T, opts Options) (Category, string) {
	t.Helper()
	return runClaudeProbes(t, context.Background(), DefaultProbes(), opts)
}

func runClaudeProbes(t *testing.T, ctx context.Context, probes []Probe, opts Options) (Category, string) {
	t.Helper()
	var buf bytes.Buffer
	cat := RunProbes(ctx, &buf, []agentsummons.ID{agentsummons.ClaudeCode}, probes, opts)
	t.Log(buf.String())
	return cat, buf.String()
}

func TestCleanRun(t *testing.T) {
	calls := stub(t, newer, nil, harnesstest.Behavior{Listing: true, Reply: harnesstest.ActivateReply})
	cat, out := runClaude(t, Options{})
	if cat != Clean {
		t.Fatalf("category = %s, want clean", cat)
	}
	for _, want := range []string{
		"probing 99.0.0 (newer than last validated " + profile.LastValidated[agentsummons.ClaudeCode] + ")",
		"probe project: discovered and loaded; body reached the model via model-output",
		"probe user: discovered and loaded",
		"probe resume: discovered and loaded",
		"clean: claude-code at 99.0.0 still matches its profile; update profile.LastValidated to 99.0.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q", want)
		}
	}
	// project + user + (warmup + activation) with no retries.
	if len(*calls) != 4 {
		t.Errorf("%d invocations, want 4", len(*calls))
	}
	// The user probe installed nothing at project scope and the fake still
	// discovered the skill, i.e. through the sandbox's user scope.
	userCall := (*calls)[1]
	if _, err := os.Stat(filepath.Join(userCall.Workdir, ".claude", "skills")); err == nil {
		t.Error("user probe installed the skill at project scope")
	}
	// Every invocation was sandboxed.
	for i, c := range *calls {
		if harnesstest.ExtraEnv(c, "CLAUDE_CONFIG_DIR") == "" {
			t.Errorf("call %d not sandboxed", i)
		}
	}
}

func TestVersionGate(t *testing.T) {
	validated := profile.LastValidated[agentsummons.ClaudeCode]
	t.Run("equal skips", func(t *testing.T) {
		calls := stub(t, validated, nil, harnesstest.Behavior{Listing: true, Reply: harnesstest.ActivateReply})
		cat, out := runClaude(t, Options{})
		if cat != Clean || !strings.Contains(out, "up to date") || len(*calls) != 0 {
			t.Errorf("cat=%s calls=%d out=%q", cat, len(*calls), out)
		}
	})
	t.Run("equal with force probes", func(t *testing.T) {
		calls := stub(t, validated, nil, harnesstest.Behavior{Listing: true, Reply: harnesstest.ActivateReply})
		cat, out := runClaude(t, Options{Force: true})
		if cat != Clean || !strings.Contains(out, "--force") || len(*calls) == 0 {
			t.Errorf("cat=%s calls=%d", cat, len(*calls))
		}
		if !strings.Contains(out, "still matches its profile\n") || strings.Contains(out, "update profile.LastValidated") {
			t.Errorf("forced run at the validated version should not ask for a table update:\n%s", out)
		}
	})
	t.Run("older skips", func(t *testing.T) {
		calls := stub(t, "0.0.1", nil, harnesstest.Behavior{})
		cat, out := runClaude(t, Options{})
		if cat != Clean || !strings.Contains(out, "older than last validated") || len(*calls) != 0 {
			t.Errorf("cat=%s calls=%d out=%q", cat, len(*calls), out)
		}
	})
}

func TestNotInstalled(t *testing.T) {
	nie := &agentsummons.NotInstalledError{Harness: agentsummons.ClaudeCode, Binary: "claude", Err: errors.New("not found")}
	t.Run("defaulted harness skips", func(t *testing.T) {
		stub(t, "", nie, harnesstest.Behavior{})
		cat, out := runClaude(t, Options{})
		if cat != Clean || !strings.Contains(out, "skipped: harness not installed") {
			t.Errorf("cat=%s out=%q", cat, out)
		}
	})
	t.Run("explicit harness errors", func(t *testing.T) {
		stub(t, "", nie, harnesstest.Behavior{})
		cat, _ := runClaude(t, Options{MissingBinaryIsError: true})
		if cat != ExecError {
			t.Errorf("cat=%s, want execution error", cat)
		}
	})
	t.Run("version probe failure errors", func(t *testing.T) {
		stub(t, "", errors.New("boom"), harnesstest.Behavior{})
		cat, out := runClaude(t, Options{})
		if cat != ExecError || !strings.Contains(out, "reading version") {
			t.Errorf("cat=%s out=%q", cat, out)
		}
	})
}

func TestUnknownHarnessIsExecError(t *testing.T) {
	var buf bytes.Buffer
	cat := RunProbes(context.Background(), &buf, []agentsummons.ID{"nope"}, DefaultProbes(), Options{})
	if cat != ExecError || !strings.Contains(buf.String(), "unsupported harness") {
		t.Errorf("cat=%s out=%q", cat, buf.String())
	}
}

// A skill the harness loads but never lists is discovery drift: the
// listing subtype the profile names no longer carries it.
func TestListingMissingIsDrift(t *testing.T) {
	stub(t, newer, nil, harnesstest.Behavior{Listing: false, Reply: harnesstest.ActivateReply})
	cat, out := runClaude(t, Options{})
	if cat != Drift {
		t.Fatalf("category = %s, want drift", cat)
	}
	for _, want := range []string{
		"probe project: drift: skill drift-project-skill absent from the discovery listing (attachment/skill_listing)",
		"to reconcile:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

// A listed skill whose body never surfaces is the model's doing, not the
// harness's: retried once, then inconclusive.
func TestBodyMissingIsInconclusive(t *testing.T) {
	calls := stub(t, newer, nil, harnesstest.Behavior{Listing: true})
	cat, out := runClaude(t, Options{})
	if cat != Inconclusive {
		t.Fatalf("category = %s, want inconclusive", cat)
	}
	if !strings.Contains(out, "retrying once") || !strings.Contains(out, "probe project: inconclusive") {
		t.Errorf("report:\n%s", out)
	}
	// project ×2, user ×2, resume ×2 turns ×2.
	if len(*calls) != 8 {
		t.Errorf("%d invocations, want 8 (each probe retried once)", len(*calls))
	}
	if !strings.Contains((*calls)[1].Prompt, "You must activate") {
		t.Errorf("retry used prompt %q", (*calls)[1].Prompt)
	}
}

func TestInvocationFailureIsExecError(t *testing.T) {
	stub(t, newer, nil, harnesstest.Behavior{})
	invoker.Run = func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		return nil, errors.New("harness exploded")
	}
	cat, out := runClaude(t, Options{})
	if cat != ExecError || !strings.Contains(out, "harness exploded") {
		t.Errorf("cat=%s out=%q", cat, out)
	}
}

// A transcript the profile cannot locate is lore drift, not an execution
// error: the fake runs fine but writes nowhere the profile looks. The
// locate wait is bounded by the context so the test does not sit out the
// profile's full flush deadline.
func TestUnlocatableTranscriptIsDrift(t *testing.T) {
	stub(t, newer, nil, harnesstest.Behavior{})
	invoker.Run = func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		return &agentsummons.Result{Harness: req.Harness, Argv: []string{"claude"}, Workdir: req.Workdir, SessionID: req.SessionID}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cat, out := runClaudeProbes(t, ctx, DefaultProbes()[:1], Options{})
	if cat != Drift || !strings.Contains(out, "transcript lore did not hold") {
		t.Errorf("cat=%s out=%q", cat, out)
	}
}

func TestKeepArchivesTranscripts(t *testing.T) {
	stub(t, newer, nil, harnesstest.Behavior{Listing: true, Reply: harnesstest.ActivateReply})
	cat, out := runClaude(t, Options{Keep: true})
	if cat != Clean {
		t.Fatalf("category = %s", cat)
	}
	marker := "kept transcripts under "
	i := strings.Index(out, marker)
	if i < 0 {
		t.Fatalf("report:\n%s", out)
	}
	dir := strings.TrimSpace(strings.SplitN(out[i+len(marker):], "\n", 2)[0])
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(dir)) })
	for _, probe := range []string{"project", "user", "resume"} {
		entries, err := os.ReadDir(filepath.Join(dir, probe))
		if err != nil || len(entries) == 0 {
			t.Errorf("probe %s: no archived transcript (%v)", probe, err)
		}
	}
}

func TestCategoryExitCodes(t *testing.T) {
	for cat, code := range map[Category]int{Clean: 0, Drift: 1, Inconclusive: 2, ExecError: 3} {
		if got := cat.ExitCode(); got != code {
			t.Errorf("%s.ExitCode() = %d, want %d", cat, got, code)
		}
	}
	if maxCategory(Inconclusive, Drift) != Drift || maxCategory(ExecError, Clean) != ExecError {
		t.Error("maxCategory does not keep the worst")
	}
	for cat, want := range map[Category]string{Clean: "clean", Drift: "drift", Inconclusive: "inconclusive", ExecError: "execution error", Category(9): "Category(9)"} {
		if got := cat.String(); got != want {
			t.Errorf("Category(%d).String() = %q, want %q", int(cat), got, want)
		}
	}
}
