package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-ecosystem/agentminutes/harness"
	"github.com/agent-ecosystem/agentsummons"
)

// codexMeta renders a codex session_meta head record.
func codexMeta(sessionID, cwd string, ts time.Time) string {
	stamp := ts.UTC().Format("2006-01-02T15:04:05.000Z")
	return fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"session_id":%q,"id":%q,"timestamp":%q,"cwd":%q,"originator":"codex_exec","cli_version":"0.144.6","source":"exec"}}`,
		stamp, sessionID, sessionID, stamp, cwd) + "\n"
}

// codexUserMessage renders a codex human user-message record.
func codexUserMessage(text string, ts time.Time) string {
	stamp := ts.UTC().Format("2006-01-02T15:04:05.000Z")
	return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`,
		stamp, text) + "\n"
}

// writeRollout writes one codex transcript into root's native date layout
// and stamps its mtime.
func writeRollout(t *testing.T, root, sessionID, content string, started, mtime time.Time) {
	t.Helper()
	day := started.UTC()
	path := filepath.Join(root,
		day.Format("2006"), day.Format("01"), day.Format("02"),
		fmt.Sprintf("rollout-%s-%s.jsonl", day.Format("2006-01-02T15-04-05"), sessionID))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// The attribution fixture clock. The run spans 10:00:05..10:00:10; with the
// profile's 5s slack the scan window is 10:00:00..10:00:15.
var (
	runStart = time.Date(2026, 7, 20, 10, 0, 5, 0, time.UTC)
	runEnd   = time.Date(2026, 7, 20, 10, 0, 10, 0, time.UTC)
)

func codexResult(t *testing.T, workdir, prompt string) *agentsummons.Result {
	t.Helper()
	return &agentsummons.Result{
		Harness:     agentsummons.Codex,
		Argv:        []string{"codex", "exec", prompt},
		PromptIndex: 2,
		Workdir:     workdir,
		Start:       runStart,
		End:         runEnd,
	}
}

func codexProfile(t *testing.T) Profile {
	t.Helper()
	p, err := For(agentsummons.Codex)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLocateOnceSingleMatch(t *testing.T) {
	p := codexProfile(t)
	root, workdir := t.TempDir(), t.TempDir()
	started := runStart.Add(time.Second)
	writeRollout(t, root, "x-1", codexMeta("x-1", workdir, started), started, runEnd.Add(2*time.Second))

	ref, err := p.locateOnce(codexResult(t, workdir, "hello"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-1" {
		t.Errorf("located %q, want x-1", ref.Meta.SessionID)
	}
}

func TestLocateOnceEmptyWindow(t *testing.T) {
	p := codexProfile(t)
	_, err := p.locateOnce(codexResult(t, t.TempDir(), "hello"), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no codex transcript in window") {
		t.Errorf("err = %v, want no-transcript-in-window", err)
	}
}

func TestLocateOnceFiltersByCWD(t *testing.T) {
	p := codexProfile(t)
	root, workdir, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	started := runStart.Add(time.Second)
	writeRollout(t, root, "x-mine", codexMeta("x-mine", workdir, started), started, runEnd.Add(time.Second))
	writeRollout(t, root, "x-other", codexMeta("x-other", elsewhere, started), started, runEnd.Add(time.Second))

	ref, err := p.locateOnce(codexResult(t, workdir, "hello"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-mine" {
		t.Errorf("located %q, want the cwd match", ref.Meta.SessionID)
	}
}

// Harnesses record the symlink-resolved cwd; the filter must match a
// transcript whose recorded cwd is the resolved form of the run's workdir.
func TestLocateOnceResolvesSymlinkedCWD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	p := codexProfile(t)
	root := t.TempDir()
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	started := runStart.Add(time.Second)
	writeRollout(t, root, "x-1", codexMeta("x-1", resolvePath(real), started), started, runEnd.Add(time.Second))

	// The run used the symlinked path; the transcript records the resolved one.
	ref, err := p.locateOnce(codexResult(t, link, "hello"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-1" {
		t.Errorf("located %q, want x-1", ref.Meta.SessionID)
	}
}

// The window matches on interval overlap, which admits a session that
// started before the run; the started-inside-window filter must reject it.
func TestLocateOncePrefersSessionStartedInWindow(t *testing.T) {
	p := codexProfile(t)
	root, workdir := t.TempDir(), t.TempDir()
	before := runStart.Add(-30 * time.Second) // overlaps via mtime, started outside
	inside := runStart.Add(time.Second)
	writeRollout(t, root, "x-old", codexMeta("x-old", workdir, before), before, runEnd.Add(time.Second))
	writeRollout(t, root, "x-new", codexMeta("x-new", workdir, inside), inside, runEnd.Add(time.Second))

	ref, err := p.locateOnce(codexResult(t, workdir, "hello"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-new" {
		t.Errorf("located %q, want the session started inside the window", ref.Meta.SessionID)
	}
}

// A transcript that stopped changing before the run began is the tail of a
// preceding run, not this run's output.
func TestLocateOnceRejectsStaleTail(t *testing.T) {
	p := codexProfile(t)
	root, workdir := t.TempDir(), t.TempDir()
	// Both started inside the window (leading slack), but the stale one's
	// mtime predates the run start.
	staleStart := runStart.Add(-4 * time.Second)
	writeRollout(t, root, "x-stale", codexMeta("x-stale", workdir, staleStart), staleStart, runStart.Add(-2*time.Second))
	liveStart := runStart.Add(time.Second)
	writeRollout(t, root, "x-live", codexMeta("x-live", workdir, liveStart), liveStart, runEnd.Add(time.Second))

	ref, err := p.locateOnce(codexResult(t, workdir, "hello"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-live" {
		t.Errorf("located %q, want the still-changing transcript", ref.Meta.SessionID)
	}
}

// When earlier filters cannot split candidates, content attribution picks
// the transcript that records the run's prompt as a human message.
func TestLocateOnceDisambiguatesByPrompt(t *testing.T) {
	p := codexProfile(t)
	root, workdir := t.TempDir(), t.TempDir()
	aStart := runStart.Add(time.Second)
	bStart := runStart.Add(2 * time.Second)
	mtime := runEnd.Add(time.Second)
	writeRollout(t, root, "x-warmup",
		codexMeta("x-warmup", workdir, aStart)+codexUserMessage("warm-up chatter", aStart),
		aStart, mtime)
	writeRollout(t, root, "x-real",
		codexMeta("x-real", workdir, bStart)+codexUserMessage("the real prompt", bStart),
		bStart, mtime)

	ref, err := p.locateOnce(codexResult(t, workdir, "the real prompt"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-real" {
		t.Errorf("located %q, want the transcript recording the prompt", ref.Meta.SessionID)
	}
}

func TestLocateOnceAmbiguous(t *testing.T) {
	p := codexProfile(t)
	root, workdir := t.TempDir(), t.TempDir()
	prompt := "same prompt"
	mtime := runEnd.Add(time.Second)
	for i, id := range []string{"x-a", "x-b"} {
		started := runStart.Add(time.Duration(i+1) * time.Second)
		writeRollout(t, root, id, codexMeta(id, workdir, started)+codexUserMessage(prompt, started), started, mtime)
	}

	_, err := p.locateOnce(codexResult(t, workdir, prompt), root)
	if err == nil || !strings.Contains(err.Error(), "cannot attribute") {
		t.Errorf("err = %v, want attribution refusal", err)
	}
}

// agyUserInput renders an antigravity USER_INPUT step carrying the prompt
// the way the harness records it (wrapped in USER_REQUEST tags).
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

// writeConversation writes one antigravity conversation into root's brain
// layout: <root>/<convID>/.system_generated/logs/transcript_full.jsonl.
func writeConversation(t *testing.T, root, convID, content string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(root, convID, ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func agyResult(t *testing.T, prompt string) *agentsummons.Result {
	t.Helper()
	return &agentsummons.Result{
		Harness:     agentsummons.Antigravity,
		Argv:        []string{"agy", prompt},
		PromptIndex: 1,
		Workdir:     t.TempDir(), // transcripts record no cwd; must not matter
		Start:       runStart,
		End:         runEnd,
	}
}

// Antigravity transcripts record no cwd, so attribution is by time window
// alone; the session ID comes from the conversation directory name.
func TestLocateOnceAntigravityByTimeWindow(t *testing.T) {
	p, err := For(agentsummons.Antigravity)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	started := runStart.Add(time.Second)
	writeConversation(t, root, "conv-real", agyUserInput(t, "hello", started), runEnd.Add(time.Second))
	// A conversation outside the window must not be a candidate.
	old := runStart.Add(-time.Hour)
	writeConversation(t, root, "conv-old", agyUserInput(t, "hello", old), old.Add(time.Minute))

	ref, err := p.locateOnce(agyResult(t, "hello"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "conv-real" {
		t.Errorf("located %q, want the conversation directory name conv-real", ref.Meta.SessionID)
	}
}

// One agy invocation writes a warm-up conversation plus the real one; only
// the real transcript records the run's prompt as a human message.
func TestLocateOnceAntigravityWarmup(t *testing.T) {
	p, err := For(agentsummons.Antigravity)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	started := runStart.Add(time.Second)
	mtime := runEnd.Add(time.Second)
	writeConversation(t, root, "conv-warmup", agyUserInput(t, "warm-up chatter", started), mtime)
	writeConversation(t, root, "conv-real", agyUserInput(t, "the real prompt", started), mtime)

	ref, err := p.locateOnce(agyResult(t, "the real prompt"), root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "conv-real" {
		t.Errorf("located %q, want the conversation recording the prompt", ref.Meta.SessionID)
	}
}

func TestLocateByIDAntigravity(t *testing.T) {
	p, err := For(agentsummons.Antigravity)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeConversation(t, root, "conv-1", agyUserInput(t, "hello", runStart), runEnd)

	ref, err := p.LocateByID(context.Background(), "conv-1", root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "conv-1" {
		t.Errorf("located %q, want conv-1", ref.Meta.SessionID)
	}
}

// ccLine renders a minimal claude-code conversation record.
func ccLine(sessionID, cwd, prompt string, ts time.Time) string {
	return fmt.Sprintf(`{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":%q},"uuid":"u-1","timestamp":%q,"userType":"external","entrypoint":"cli","cwd":%q,"sessionId":%q,"version":"2.1.205","gitBranch":"main"}`,
		prompt, ts.UTC().Format("2006-01-02T15:04:05.000Z"), cwd, sessionID) + "\n"
}

func TestLocateClaudeCodeByPresetID(t *testing.T) {
	p, err := For(agentsummons.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	root, workdir := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "-proj", "s-run.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(ccLine("s-run", workdir, "hello", runStart)), 0o644); err != nil {
		t.Fatal(err)
	}

	res := &agentsummons.Result{Harness: agentsummons.ClaudeCode, Workdir: workdir, Start: runStart, End: runEnd, SessionID: "s-run"}
	ref, err := p.locateOnce(res, root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "s-run" || ref.Path != path {
		t.Errorf("ref = %+v, want s-run at %s", ref, path)
	}
}

func TestLocateByID(t *testing.T) {
	p := codexProfile(t)
	root := t.TempDir()
	started := runStart.Add(time.Second)
	writeRollout(t, root, "x-1", codexMeta("x-1", "/tmp/exp", started), started, runEnd)

	ref, err := p.LocateByID(context.Background(), "x-1", root)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Meta.SessionID != "x-1" {
		t.Errorf("located %q, want x-1", ref.Meta.SessionID)
	}
}

// The retry loops must honor cancellation instead of spinning out their
// full 15-second deadline.
func TestLocateRetriesHonorCancellation(t *testing.T) {
	p := codexProfile(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	begin := time.Now()
	_, err := p.LocateByID(ctx, "x-none", t.TempDir())
	if err == nil {
		t.Fatal("want error for canceled locate")
	}
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Errorf("canceled locate took %v", elapsed)
	}

	_, err = p.Locate(ctx, codexResult(t, t.TempDir(), "hello"), t.TempDir())
	if err == nil {
		t.Fatal("want error for canceled locate")
	}
}

func TestTranscriptHasPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-x.jsonl")
	content := codexMeta("x-1", "/tmp/exp", runStart) + codexUserMessage("find the treasure", runStart)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if !transcriptHasPrompt(harness.Codex, path, "find the treasure") {
		t.Error("recorded prompt not found")
	}
	if transcriptHasPrompt(harness.Codex, path, "some other prompt") {
		t.Error("absent prompt reported found")
	}

	// Attribution must never crown an unreadable candidate.
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if transcriptHasPrompt(harness.Codex, bad, "anything") {
		t.Error("unparseable transcript reported a match")
	}
	if transcriptHasPrompt(harness.Codex, filepath.Join(dir, "missing.jsonl"), "anything") {
		t.Error("missing transcript reported a match")
	}
}
