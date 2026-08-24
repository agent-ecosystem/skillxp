package profile

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/agent-ecosystem/agentsummons"
)

func TestForKnownAndUnknown(t *testing.T) {
	for _, p := range Profiles() {
		got, err := For(p.Harness)
		if err != nil {
			t.Errorf("For(%s): %v", p.Harness, err)
		}
		if got.Harness != p.Harness {
			t.Errorf("For(%s) returned profile for %s", p.Harness, got.Harness)
		}
	}
	if _, err := For(agentsummons.ID("not-a-harness")); err == nil {
		t.Error("For(unknown): want error")
	}
}

func TestProfilesInvariants(t *testing.T) {
	profiles := Profiles()
	if len(profiles) == 0 {
		t.Fatal("no profiles")
	}
	if !sort.SliceIsSorted(profiles, func(i, j int) bool {
		return profiles[i].Harness < profiles[j].Harness
	}) {
		t.Error("profiles are documented alphabetical but are not sorted")
	}
	for _, p := range profiles {
		if p.ProjectSkillDir == "" {
			t.Errorf("%s: empty ProjectSkillDir", p.Harness)
		}
		if p.LocateSlack <= 0 {
			t.Errorf("%s: LocateSlack = %v, want positive", p.Harness, p.LocateSlack)
		}
		if p.EchoSubtypes == nil {
			t.Errorf("%s: nil EchoSubtypes (trace callers index it unconditionally)", p.Harness)
		}
		// A harness that records no injected context has no trustworthy
		// listing location by definition.
		if !p.RecordsInjectedContext && len(p.SkillListingSubtypes) > 0 {
			t.Errorf("%s: claims listing subtypes %v without recording injected context", p.Harness, p.SkillListingSubtypes)
		}
	}
}

func TestActivationPrompt(t *testing.T) {
	p := Profile{Harness: agentsummons.ClaudeCode}
	got := p.ActivationPrompt("my-skill")
	if !strings.Contains(got, "my-skill") {
		t.Errorf("prompt %q does not name the skill", got)
	}
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestPrepareIdentity(t *testing.T) {
	for _, p := range Profiles() {
		req := agentsummons.Request{Harness: p.Harness}
		if err := p.Prepare(&req, false); err != nil {
			t.Fatalf("%s: %v", p.Harness, err)
		}
		if p.Harness == agentsummons.ClaudeCode {
			if !uuidV4.MatchString(req.SessionID) {
				t.Errorf("claude-code preset session id %q is not a v4 UUID", req.SessionID)
			}
		} else if req.SessionID != "" {
			t.Errorf("%s: Prepare preset a session id %q; only claude-code supports that", p.Harness, req.SessionID)
		}
		if req.Resume != "" {
			t.Errorf("%s: Prepare set Resume %q on an opening turn", p.Harness, req.Resume)
		}
	}
}

// SessionID and Resume are competing identity claims; a resume turn must
// carry only the resume ref.
func TestPrepareResumeNeverPresetsIdentity(t *testing.T) {
	for _, p := range Profiles() {
		req := agentsummons.Request{Harness: p.Harness}
		p.PrepareResume(&req, true, "session-123")
		if req.Resume != "session-123" {
			t.Errorf("%s: Resume = %q, want session-123", p.Harness, req.Resume)
		}
		if req.SessionID != "" {
			t.Errorf("%s: resume turn preset SessionID %q", p.Harness, req.SessionID)
		}
	}
}

func TestPermissions(t *testing.T) {
	perms := func(id agentsummons.ID, activation bool) agentsummons.Request {
		p, err := For(id)
		if err != nil {
			t.Fatal(err)
		}
		req := agentsummons.Request{Harness: id}
		p.permissions(&req, activation)
		return req
	}

	if req := perms(agentsummons.ClaudeCode, true); len(req.AllowedTools) != 1 || req.AllowedTools[0] != "Skill" {
		t.Errorf("claude-code activation AllowedTools = %v, want [Skill]", req.AllowedTools)
	}
	if req := perms(agentsummons.Antigravity, true); !req.AutoApprove {
		t.Error("antigravity activation must set AutoApprove")
	}
	if req := perms(agentsummons.Codex, true); req.AutoApprove || len(req.AllowedTools) != 0 {
		t.Errorf("codex activation needs no extra permissions, got %+v", req)
	}

	// Passive turns get no tool permissions on any harness.
	for _, p := range Profiles() {
		if req := perms(p.Harness, false); req.AutoApprove || len(req.AllowedTools) != 0 {
			t.Errorf("%s: passive turn granted permissions: %+v", p.Harness, req)
		}
	}
}

func TestParseOptions(t *testing.T) {
	for _, p := range Profiles() {
		opts := p.ParseOptions("9.9.9")
		if p.RecordsInjectedContext && opts.HarnessVersionHint != "" {
			t.Errorf("%s: version hint %q set for a harness that records versions in-band", p.Harness, opts.HarnessVersionHint)
		}
		if !p.RecordsInjectedContext && opts.HarnessVersionHint != "9.9.9" {
			t.Errorf("%s: version hint = %q, want the CLI version", p.Harness, opts.HarnessVersionHint)
		}
	}
}

func TestNewUUID(t *testing.T) {
	a, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if !uuidV4.MatchString(a) {
		t.Errorf("%q is not a v4 UUID", a)
	}
	b, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two UUIDs collided")
	}
}

func writeSkill(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "my-skill")
	if err := os.MkdirAll(filepath.Join(src, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: my-skill\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "references", "extra.md"), []byte("extra\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestInstallSkill(t *testing.T) {
	p, err := For(agentsummons.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	src := writeSkill(t, t.TempDir())
	project := t.TempDir()
	if err := p.InstallSkill(project, src); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(project, p.ProjectSkillDir, "my-skill")
	data, err := os.ReadFile(filepath.Join(installed, "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not installed: %v", err)
	}
	if !strings.Contains(string(data), "my-skill") {
		t.Errorf("installed SKILL.md content = %q", data)
	}
	if _, err := os.Stat(filepath.Join(installed, "references", "extra.md")); err != nil {
		t.Errorf("nested file not copied: %v", err)
	}
}

func TestInstallSkillTo(t *testing.T) {
	p, err := For(agentsummons.Codex)
	if err != nil {
		t.Fatal(err)
	}
	src := writeSkill(t, t.TempDir())
	dest := t.TempDir()
	if err := p.InstallSkillTo(dest, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "my-skill", "SKILL.md")); err != nil {
		t.Errorf("skill not installed under destDir/basename: %v", err)
	}
}

func TestCopyTreePreservesModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not faithful on windows")
	}
	src := writeSkill(t, t.TempDir())
	dest := filepath.Join(t.TempDir(), "copy")
	if err := copyTree(src, dest); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dest, "references", "extra.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("copied mode = %o, want 600", got)
	}
}

func TestCopyTreeMissingSource(t *testing.T) {
	if err := copyTree(filepath.Join(t.TempDir(), "nope"), t.TempDir()); err == nil {
		t.Error("want error for missing source")
	}
}

func TestPrepareSandboxClaudeCode(t *testing.T) {
	p, err := For(agentsummons.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	if _, err := p.PrepareSandbox(t.TempDir()); err == nil {
		t.Error("missing token: want error")
	}

	// An unresolved 1Password reference would be sent verbatim and 401;
	// the sandbox must refuse it loudly instead.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "op://vault/item/token")
	if _, err := p.PrepareSandbox(t.TempDir()); err == nil || !strings.Contains(err.Error(), "1Password") {
		t.Errorf("op:// token: got %v, want the 1Password guidance", err)
	}

	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-fake")
	fixture := t.TempDir()
	sb, err := p.PrepareSandbox(fixture)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(fixture, "home", "claude")
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("sandbox config dir not created: %v", err)
	}
	wantEnv := map[string]bool{"CLAUDE_CONFIG_DIR=" + dir: true, "ANTHROPIC_API_KEY=": true}
	for _, e := range sb.Env {
		delete(wantEnv, e)
	}
	if len(wantEnv) != 0 {
		t.Errorf("sandbox env %v missing %v", sb.Env, wantEnv)
	}
	if sb.TranscriptRoot != filepath.Join(dir, "projects") || sb.UserSkillDir != filepath.Join(dir, "skills") {
		t.Errorf("sandbox roots = %q / %q", sb.TranscriptRoot, sb.UserSkillDir)
	}
}

// fakeHome points SeedRoot/UserHomeDir at a fresh temp dir.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // windows
	return home
}

func TestPrepareSandboxCodexFallsBackToRealAuth(t *testing.T) {
	home := fakeHome(t)
	p, err := For(agentsummons.Codex)
	if err != nil {
		t.Fatal(err)
	}
	// No seed: auth.json (and config.toml when present) copy from ~/.codex.
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	fixture := t.TempDir()
	sb, err := p.PrepareSandbox(fixture)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(fixture, "home", "codex")
	data, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil || string(data) != `{"token":"x"}` {
		t.Errorf("auth.json = %q, %v; want copy of the real one", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err == nil {
		t.Error("config.toml materialized from nothing")
	}
	if len(sb.Env) != 1 || sb.Env[0] != "CODEX_HOME="+dir {
		t.Errorf("env = %v, want CODEX_HOME override", sb.Env)
	}
	if sb.TranscriptRoot != filepath.Join(dir, "sessions") {
		t.Errorf("transcript root = %q", sb.TranscriptRoot)
	}
}

func TestPrepareSandboxCodexPrefersSeed(t *testing.T) {
	home := fakeHome(t)
	p, err := For(agentsummons.Codex)
	if err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(home, ".skillxp", "seeds", "codex")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "auth.json"), []byte(`{"token":"seeded"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// A real ~/.codex also exists; the seed must win.
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"token":"real"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	fixture := t.TempDir()
	if _, err := p.PrepareSandbox(fixture); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(fixture, "home", "codex", "auth.json"))
	if err != nil || string(data) != `{"token":"seeded"}` {
		t.Errorf("auth.json = %q, %v; want the seed copy", data, err)
	}
}

func TestPrepareSandboxAntigravity(t *testing.T) {
	home := fakeHome(t)
	p, err := For(agentsummons.Antigravity)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.PrepareSandbox(t.TempDir()); err == nil {
		t.Error("missing seed: want error with setup instructions")
	}

	seed := filepath.Join(home, ".skillxp", "seeds", "antigravity", "home")
	if err := os.MkdirAll(filepath.Join(seed, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, ".gemini", "token"), []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	sb, err := p.PrepareSandbox(fixture)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(fixture, "home", "agy")
	if _, err := os.Stat(filepath.Join(dir, ".gemini", "token")); err != nil {
		t.Errorf("seed not cloned: %v", err)
	}
	wantEnv := map[string]bool{"HOME=" + dir: true, "BROWSER=false": true}
	for _, e := range sb.Env {
		delete(wantEnv, e)
	}
	if len(wantEnv) != 0 {
		t.Errorf("sandbox env %v missing %v", sb.Env, wantEnv)
	}
}

func TestPrepareSandboxUnknownHarness(t *testing.T) {
	p := Profile{Harness: agentsummons.ID("not-a-harness")}
	if _, err := p.PrepareSandbox(t.TempDir()); err == nil {
		t.Error("want error for unsupported harness")
	}
}

func TestResolvePathFallsBack(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	if got := resolvePath(missing); got != missing {
		t.Errorf("resolvePath(%q) = %q, want the input back", missing, got)
	}
}
