// Package profile owns the per-harness lore skill invocation experiments
// need beyond what agentsummons and agentminutes model: where each harness
// discovers installed skills, how to activate one headlessly, how to find
// the transcript a run produced, and which normalized-event locations are
// trustworthy evidence versus echoes of the prompt.
//
// Everything here was established empirically (see the spike notes in the
// README); each field cites the harness version it was validated against.
package profile

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-ecosystem/agentminutes"
	"github.com/agent-ecosystem/agentminutes/harness"
	"github.com/agent-ecosystem/agentminutes/session"
	"github.com/agent-ecosystem/agentsummons"
)

// Profile is the benchmark's knowledge about one harness.
type Profile struct {
	Harness agentsummons.ID

	// ProjectSkillDir is where the harness discovers project-level skills,
	// relative to the project root.
	ProjectSkillDir string

	// UserSkillDir is where the harness discovers user-level skills,
	// relative to the user's home directory. Empty means not yet
	// established.
	UserSkillDir string

	// SkillListingSubtypes are the system-event subtypes that carry the
	// harness's skill discovery listing in a normalized transcript. Empty
	// means the harness does not record injected context at all
	// (Antigravity), so discovery evidence must be inferred behaviorally.
	SkillListingSubtypes []string

	// EchoSubtypes are system-event subtypes that merely replay
	// conversation content (prompts, final answers). A phrase appearing
	// only there is an echo, not loading evidence. Established per harness
	// during the spike: codex task_complete carries the final agent
	// message; antigravity user_input_context and conversation_history
	// replay the prompt.
	EchoSubtypes map[string]bool

	// RecordsInjectedContext reports whether the native transcript records
	// harness-injected context (system prompt, skill listing, injected
	// skill bodies). When false, findings that depend on injected content
	// carry inferred confidence at best.
	RecordsInjectedContext bool

	// LocateSlack pads the run's [start, end] window when locating the
	// transcript by time.
	LocateSlack time.Duration
}

// Profiles returns the supported harness profiles, alphabetical. Validated
// against: antigravity 1.1.4, claude-code 2.1.205, codex 0.144.6.
func Profiles() []Profile {
	return []Profile{
		{
			Harness:         agentsummons.Antigravity,
			ProjectSkillDir: filepath.Join(".agents", "skills"),
			// Validated 1.1.9 (2026-08-01): global skills live at
			// ~/.gemini/config/skills, per the docs and confirmed by
			// listing probes. The 1.1.4-era locations
			// (~/.gemini/antigravity-cli/skills, ~/.gemini/skills) exist
			// on disk but no longer reach the listing, and a HOME-level
			// .agents/skills never worked — the .agents root is
			// project-only.
			UserSkillDir:           filepath.Join(".gemini", "config", "skills"),
			SkillListingSubtypes:   nil,
			EchoSubtypes:           map[string]bool{"user_input_context": true, "conversation_history": true, "checkpoint": true},
			RecordsInjectedContext: false,
			LocateSlack:            5 * time.Second,
		},
		{
			Harness:                agentsummons.ClaudeCode,
			ProjectSkillDir:        filepath.Join(".claude", "skills"),
			UserSkillDir:           filepath.Join(".claude", "skills"),
			SkillListingSubtypes:   []string{"attachment/skill_listing"},
			EchoSubtypes:           map[string]bool{},
			RecordsInjectedContext: true,
			LocateSlack:            5 * time.Second,
		},
		{
			Harness:                agentsummons.Codex,
			ProjectSkillDir:        filepath.Join(".codex", "skills"),
			UserSkillDir:           filepath.Join(".codex", "skills"),
			SkillListingSubtypes:   []string{"message/developer"},
			EchoSubtypes:           map[string]bool{"task_complete": true},
			RecordsInjectedContext: true,
			LocateSlack:            5 * time.Second,
		},
	}
}

// For returns the profile for one harness.
func For(id agentsummons.ID) (Profile, error) {
	for _, p := range Profiles() {
		if p.Harness == id {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("profile: unsupported harness %q", id)
}

// Sandbox is an isolated home for one run: the environment overrides that
// point the harness at it, where transcripts land inside it, and where
// user-scope skills install.
type Sandbox struct {
	// Env is appended to the request environment; entries override any
	// parent values of the same name.
	Env []string

	// TranscriptRoot is where the harness writes transcripts inside the
	// sandbox; locate calls scan here instead of the default root.
	TranscriptRoot string

	// UserSkillDir is the absolute user-scope skill directory inside the
	// sandbox.
	UserSkillDir string
}

// SeedRoot is the per-harness sandbox seed location. Seeds hold the
// minimum authenticated state a sandboxed harness needs; PrepareSandbox
// documents each harness's one-time setup.
func SeedRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".skillxp", "seeds"), nil
}

// PrepareSandbox builds an isolated home under fixtureDir and returns how
// to use it. Auth material per harness (all validated empirically):
// codex clones ~/.skillxp/seeds/codex when present, else copies auth.json
// and config.toml from the real ~/.codex; claude-code needs
// CLAUDE_CODE_OAUTH_TOKEN in the environment (macOS keychain credentials
// are unreachable from a sandboxed CLAUDE_CONFIG_DIR); antigravity clones
// a seed home the user authenticated once.
func (p Profile) PrepareSandbox(fixtureDir string) (*Sandbox, error) {
	seedRoot, err := SeedRoot()
	if err != nil {
		return nil, err
	}
	switch p.Harness {
	case agentsummons.ClaudeCode:
		switch token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); {
		case token == "":
			return nil, fmt.Errorf("profile: claude-code sandbox needs CLAUDE_CODE_OAUTH_TOKEN in the environment; mint one with `claude setup-token` (bills to your subscription, not the API)")
		case strings.HasPrefix(token, "op://"):
			// A 1Password secret reference reached us unresolved; the
			// harness would send it verbatim and get a baffling 401.
			return nil, fmt.Errorf("profile: CLAUDE_CODE_OAUTH_TOKEN is an unresolved 1Password reference; run under `op run -- <command>` so it resolves")
		}
		dir := filepath.Join(fixtureDir, "home", "claude")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		return &Sandbox{
			// ANTHROPIC_API_KEY is emptied so a stray key in the parent
			// environment cannot outrank the subscription token in
			// claude-code's auth precedence and silently switch the run
			// to metered API billing.
			Env:            []string{"CLAUDE_CONFIG_DIR=" + dir, "ANTHROPIC_API_KEY="},
			TranscriptRoot: filepath.Join(dir, "projects"),
			UserSkillDir:   filepath.Join(dir, "skills"),
		}, nil
	case agentsummons.Codex:
		dir := filepath.Join(fixtureDir, "home", "codex")
		seed := filepath.Join(seedRoot, "codex")
		if _, err := os.Stat(seed); err == nil {
			if err := copyTree(seed, dir); err != nil {
				return nil, err
			}
		} else {
			real, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
			// auth.json is the only required material (validated
			// 0.144.6); config.toml rides along when present.
			for _, f := range []string{"auth.json", "config.toml"} {
				data, err := os.ReadFile(filepath.Join(real, ".codex", f))
				if err != nil {
					continue
				}
				if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
					return nil, err
				}
			}
		}
		return &Sandbox{
			Env:            []string{"CODEX_HOME=" + dir},
			TranscriptRoot: filepath.Join(dir, "sessions"),
			UserSkillDir:   filepath.Join(dir, "skills"),
		}, nil
	case agentsummons.Antigravity:
		seed := filepath.Join(seedRoot, "antigravity", "home")
		if _, err := os.Stat(seed); err != nil {
			return nil, fmt.Errorf("profile: antigravity sandbox needs a one-time authenticated seed home: run `mkdir -p %s && HOME=%s agy`, complete the login it prompts for, then quit; if macOS reports a missing keychain, click Cancel — agy falls back to a token file inside the seed, which is exactly what sandbox clones need", seed, seed)
		}
		dir := filepath.Join(fixtureDir, "home", "agy")
		if err := copyTree(seed, dir); err != nil {
			return nil, err
		}
		return &Sandbox{
			// BROWSER=false is a best-effort guard: if auth state fails
			// to transplant, a re-auth attempt should fail in text
			// rather than hijack the user's browser.
			Env:            []string{"HOME=" + dir, "BROWSER=false"},
			TranscriptRoot: filepath.Join(dir, ".gemini", "antigravity-cli", "brain"),
			UserSkillDir:   filepath.Join(dir, ".gemini", "config", "skills"),
		}, nil
	}
	return nil, fmt.Errorf("profile: unsupported harness %q", p.Harness)
}

// ActivationPrompt is the natural-language activation trigger validated on
// all three harnesses. Slash-command and other activation styles are a
// future check dimension, not a runner default.
func (p Profile) ActivationPrompt(skill string) string {
	return fmt.Sprintf("Activate the %s skill and follow its instructions.", skill)
}

// Prepare applies harness-specific request settings for an opening turn.
// Activation turns get the minimal permissions that let the harness's
// loading mechanism run: claude-code needs its Skill tool allowed,
// antigravity needs permission bypass for its view_file pull. Passive turns
// (phrase interrogation) get no tool permissions at all.
func (p Profile) Prepare(req *agentsummons.Request, activation bool) error {
	if p.Harness == agentsummons.ClaudeCode {
		// Preset identity so the transcript is addressable before the run.
		id, err := newUUID()
		if err != nil {
			return err
		}
		req.SessionID = id
	}
	p.permissions(req, activation)
	return nil
}

// PrepareResume applies harness-specific settings for a follow-up turn in
// an existing session. sessionID is the ref established by the opening
// turn; identity presets are never combined with resume — they are
// competing identity claims.
func (p Profile) PrepareResume(req *agentsummons.Request, activation bool, sessionID string) {
	req.Resume = sessionID
	p.permissions(req, activation)
}

func (p Profile) permissions(req *agentsummons.Request, activation bool) {
	if !activation {
		return
	}
	switch p.Harness {
	case agentsummons.ClaudeCode:
		req.AllowedTools = []string{"Skill"}
	case agentsummons.Antigravity:
		req.AutoApprove = true
	case agentsummons.Codex:
		// Default read-only sandbox covers activation; codex's model-pull
		// only needs file reads.
	}
}

// Locate finds the transcript a finished run wrote, waiting briefly for the
// harness to flush it. root overrides the harness's default transcript
// root (a sandbox's TranscriptRoot); empty means the default. Strategy per
// harness: claude-code resolves the preset session ID directly; codex
// scans filtered by the run's cwd and time window; antigravity scans by
// time window alone (its transcripts record no cwd).
func (p Profile) Locate(ctx context.Context, res *agentsummons.Result, root string) (harness.SessionRef, error) {
	deadline := time.Now().Add(15 * time.Second)
	for {
		ref, err := p.locateOnce(res, root)
		if err == nil {
			return ref, nil
		}
		if time.Now().After(deadline) {
			return harness.SessionRef{}, err
		}
		select {
		case <-ctx.Done():
			return harness.SessionRef{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// LocateByID resolves a known session ID to its transcript, waiting
// briefly for the harness to flush. root overrides the default transcript
// root; empty means the default. Resume turns append to the same session
// on every supported harness (validated: claude-code 2.1.205, codex
// 0.144.6, antigravity 1.1.4), so a follow-up turn's transcript is always
// reachable by the opening turn's ID.
func (p Profile) LocateByID(ctx context.Context, sessionID, root string) (harness.SessionRef, error) {
	l, err := agentminutes.LocatorFor(harness.ID(p.Harness))
	if err != nil {
		return harness.SessionRef{}, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		ref, err := locateIn(l, root, sessionID)
		if err == nil {
			return ref, nil
		}
		if time.Now().After(deadline) {
			return harness.SessionRef{}, err
		}
		select {
		case <-ctx.Done():
			return harness.SessionRef{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func locateIn(l harness.Locator, root, sessionID string) (harness.SessionRef, error) {
	if root == "" {
		var err error
		root, err = l.DefaultRoot()
		if err != nil {
			return harness.SessionRef{}, err
		}
	}
	return l.Locate(root, sessionID)
}

func (p Profile) locateOnce(res *agentsummons.Result, root string) (harness.SessionRef, error) {
	hid := harness.ID(p.Harness)
	l, err := agentminutes.LocatorFor(hid)
	if err != nil {
		return harness.SessionRef{}, err
	}
	if p.Harness == agentsummons.ClaudeCode {
		return locateIn(l, root, res.SessionID)
	}
	if root == "" {
		root, err = l.DefaultRoot()
		if err != nil {
			return harness.SessionRef{}, err
		}
	}
	since := res.Start.Add(-p.LocateSlack)
	until := res.End.Add(p.LocateSlack)
	opts := harness.ScanOptions{Since: since, Until: until}
	// Harnesses record the symlink-resolved cwd (macOS /var/folders vs
	// /private/var/folders); compare resolved paths.
	wantCWD := resolvePath(res.Workdir)
	var matches []harness.SessionRef
	for ref, err := range l.Scan(root, opts) {
		if err != nil {
			continue // per-file scan errors don't invalidate other refs
		}
		if p.Harness == agentsummons.Codex && resolvePath(ref.Meta.CWD) != wantCWD {
			continue
		}
		matches = append(matches, ref)
	}
	if len(matches) > 1 {
		// The scan window matches on interval overlap, which admits the
		// tail of an immediately preceding run. Require the session to
		// have started inside the window.
		var started []harness.SessionRef
		for _, ref := range matches {
			if ref.StartedAt != nil && !ref.StartedAt.Before(since) && !ref.StartedAt.After(until) {
				started = append(started, ref)
			}
		}
		if len(started) > 0 {
			matches = started
		}
	}
	if len(matches) > 1 {
		// A transcript that stopped changing before this run began cannot
		// be this run's output: the window's leading slack admits the tail
		// of the immediately preceding run, and when both runs used the
		// same prompt (common across benchmark checks) the prompt filter
		// below cannot split them. ModTime is a proxy for session end; the
		// runner scans live transcript roots, never archives, so it is
		// honest here.
		var active []harness.SessionRef
		for _, ref := range matches {
			if !ref.ModTime.Before(res.Start) {
				active = append(active, ref)
			}
		}
		if len(active) > 0 {
			matches = active
		}
	}
	if len(matches) > 1 {
		// One agy invocation writes two conversations: a warm-up plus the
		// real one (observed on 1.1.4). Attribute by content: the real
		// transcript records our prompt as a human user message.
		prompt := res.Argv[res.PromptIndex]
		var withPrompt []harness.SessionRef
		for _, ref := range matches {
			if transcriptHasPrompt(hid, ref.Path, prompt) {
				withPrompt = append(withPrompt, ref)
			}
		}
		if len(withPrompt) > 0 {
			matches = withPrompt
		}
	}
	switch len(matches) {
	case 0:
		return harness.SessionRef{}, fmt.Errorf("profile: no %s transcript in window %s..%s", p.Harness, since.Format(time.RFC3339), until.Format(time.RFC3339))
	case 1:
		return matches[0], nil
	default:
		// Concurrent sessions in the window make attribution unsafe; the
		// runner serializes runs precisely so this stays exceptional.
		return harness.SessionRef{}, fmt.Errorf("profile: %d candidate %s transcripts in window; cannot attribute run", len(matches), p.Harness)
	}
}

// ParseOptions returns the agentminutes options for parsing this harness's
// transcript. cliVersion is stamped as a hint for formats that record no
// version in-band (Antigravity).
func (p Profile) ParseOptions(cliVersion string) harness.Options {
	opts := harness.Options{}
	if !p.RecordsInjectedContext {
		opts.HarnessVersionHint = cliVersion
	}
	return opts
}

// InstallSkill copies one skill directory into the project-level skill
// location for this harness.
func (p Profile) InstallSkill(projectDir, skillSource string) error {
	dest := filepath.Join(projectDir, p.ProjectSkillDir, filepath.Base(skillSource))
	return copyTree(skillSource, dest)
}

// InstallSkillTo copies one skill directory into an absolute skills
// directory (e.g. a sandbox's user scope).
func (p Profile) InstallSkillTo(destDir, skillSource string) error {
	return copyTree(skillSource, filepath.Join(destDir, filepath.Base(skillSource)))
}

// transcriptHasPrompt reports whether the transcript records the given
// prompt as a human-origin user message. Parse failures count as no match:
// attribution must never crown an unreadable candidate.
func transcriptHasPrompt(hid harness.ID, path, prompt string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	s, err := agentminutes.Parse(f, hid, harness.Options{Permissive: true})
	if err != nil {
		return false
	}
	for i := range s.Events {
		ev := &s.Events[i]
		if ev.Kind == session.KindUserMessage && ev.UserMessage.Origin == session.OriginHuman &&
			strings.Contains(ev.UserMessage.Text(), prompt) {
			return true
		}
	}
	return false
}

// resolvePath returns the symlink-resolved form of path, or path itself
// when resolution fails (e.g. the directory was already cleaned up).
func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// newUUID returns a random v4 UUID string.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
