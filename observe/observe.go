// Package observe runs skill invocation experiments and returns
// everything observable about them: fixture setup, headless invocation via
// agentsummons, transcript location and parsing via agentminutes, and
// optional archival. It renders no verdicts — consumers (benchmark
// runners, skill CI, eval harnesses) grade the observations themselves,
// typically with the trace package.
package observe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-ecosystem/agentminutes"
	"github.com/agent-ecosystem/agentminutes/harness"
	"github.com/agent-ecosystem/agentminutes/session"
	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/internal/invoker"
	"github.com/agent-ecosystem/skillxp/profile"
	"github.com/agent-ecosystem/skillxp/trace"
)

// Turn is one prompt in a session.
type Turn struct {
	// Prompt is the turn's prompt. Callers planning to trace phrases
	// should never put a traced phrase in the prompt: a literal-minded
	// model truthfully answers "it's in your message", and the phrase
	// contaminates every echo location in the transcript.
	Prompt string

	// Activation marks the turn as a skill-activation turn, which gets
	// the harness's minimal activation permissions (profile.Prepare).
	Activation bool

	// Before, when set, runs immediately before this turn's invocation
	// with the fixture's project directory — the hook for editing skill
	// files on disk between turns (e.g. reactivation freshness
	// experiments).
	Before func(projectDir string) error
}

// Spec describes a single-turn experiment.
type Spec struct {
	// SkillDirs are paths to skill directories to install at project
	// scope in the fixture before the run.
	SkillDirs []string

	// Prompt and Activation form the session's only turn; see Turn.
	Prompt     string
	Activation bool
}

// SessionSpec describes a multi-turn experiment: one fixture, one harness
// session, each turn after the first resuming it.
type SessionSpec struct {
	// SkillDirs are paths to skill directories to install at project
	// scope in the fixture before the first turn.
	SkillDirs []string

	// UserSkillDirs are paths to skill directories to install at USER
	// scope. Requires Config.Sandbox (installing into the real user scope
	// would pollute the machine), and a harness whose sandbox models a
	// user scope.
	UserSkillDirs []string

	// ProjectDirs are paths to directory trees copied into the fixture's
	// project ROOT under their basenames — NOT the harness's skill
	// directory. For fixtures that need non-skill files, e.g. a stray
	// SKILL.md outside any recognized skills root.
	ProjectDirs []string

	// OverlayDirs are paths to directory trees whose CONTENTS are copied
	// onto the fixture's project root, preserving their internal layout.
	// For fixtures that must land at exact paths the basename rule cannot
	// express (e.g. .agents/skills/<skill>/ on a harness whose native
	// skill dir is elsewhere).
	OverlayDirs []string

	// Turns run in order against the same session.
	Turns []Turn
}

// Config controls execution.
type Config struct {
	// Timeout bounds each harness invocation. Zero means 5 minutes.
	Timeout time.Duration

	// Sandbox runs the harness against an isolated home built from the
	// profile's seed material (profile.PrepareSandbox), so user-level
	// state can neither leak into the experiment nor be polluted by it.
	Sandbox bool

	// KeepFixture leaves the fixture directory on disk.
	KeepFixture bool

	// ArchiveDir, when set, receives copies of the final transcript(s) so
	// evidence line numbers stay citable after harness stores rotate.
	ArchiveDir string

	// Log receives progress lines; nil silences them.
	Log func(format string, args ...any)
}

func (c Config) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(format, args...)
	}
}

// Observation is everything observable about one finished turn. Sessions
// accumulate: a turn's Session is the parse of the whole transcript as of
// that turn, not a delta.
type Observation struct {
	Harness        string    `json:"harness"`
	CLIVersion     string    `json:"cli_version"`
	HarnessVersion string    `json:"harness_version"`
	Model          string    `json:"model,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	Prompt         string    `json:"prompt"`
	Activation     bool      `json:"activation"`
	Started        time.Time `json:"started"`
	Ended          time.Time `json:"ended"`
	ExitCode       int       `json:"exit_code"`
	Sandboxed      bool      `json:"sandboxed,omitempty"`

	// TranscriptPath is the archived copy when ArchiveDir was set (final
	// turn only), else the native path in the harness's own store — which
	// for sandboxed runs is inside the fixture and vanishes with it
	// unless archived or kept.
	TranscriptPath string   `json:"transcript_path"`
	SubagentPaths  []string `json:"subagent_paths,omitempty"`

	// Session is the normalized parse of the transcript. Serialized
	// separately from this struct (it has its own schema contract).
	Session *session.Session `json:"-"`

	Profile profile.Profile      `json:"-"`
	Result  *agentsummons.Result `json:"-"`
	Ref     harness.SessionRef   `json:"-"`
}

// SessionObservation is a completed multi-turn experiment.
type SessionObservation struct {
	Harness   string `json:"harness"`
	SessionID string `json:"session_id"`

	// ProjectDir is the fixture's project directory; it survives only
	// when Config.KeepFixture was set.
	ProjectDir string `json:"project_dir,omitempty"`

	// Turns holds one observation per turn, in order. The final turn's
	// Session covers the whole conversation.
	Turns []*Observation `json:"turns"`
}

// Final returns the last turn's observation.
func (so *SessionObservation) Final() *Observation {
	return so.Turns[len(so.Turns)-1]
}

// Observe runs a single-turn experiment. See ObserveSession.
func Observe(ctx context.Context, cfg Config, harnessID agentsummons.ID, spec Spec) (*Observation, error) {
	so, err := ObserveSession(ctx, cfg, harnessID, SessionSpec{
		SkillDirs: spec.SkillDirs,
		Turns:     []Turn{{Prompt: spec.Prompt, Activation: spec.Activation}},
	})
	if err != nil {
		return nil, err
	}
	return so.Final(), nil
}

// ObserveSession runs a multi-turn experiment against one harness. The
// fixture is a fresh project directory with the spec's skills installed at
// project scope; every turn after the first resumes the session the first
// turn opened (all supported harnesses append resumed turns to the same
// transcript). Callers running multiple experiments must serialize them:
// concurrent sessions break time-window transcript attribution and
// cross-contaminate context-sensitive observations.
func ObserveSession(ctx context.Context, cfg Config, harnessID agentsummons.ID, spec SessionSpec) (*SessionObservation, error) {
	if len(spec.Turns) == 0 {
		return nil, fmt.Errorf("observe: session has no turns")
	}
	p, err := profile.For(harnessID)
	if err != nil {
		return nil, err
	}
	// Validate the spec before spending a harness probe on it.
	if len(spec.UserSkillDirs) > 0 && !cfg.Sandbox {
		return nil, fmt.Errorf("observe: user-scope installs require Config.Sandbox (the real user scope is never touched)")
	}
	cliVersion, err := invoker.Version(ctx, harnessID)
	if err != nil {
		return nil, fmt.Errorf("observe: %s not usable: %w", harnessID, err)
	}

	// Fixture.
	fixture, err := os.MkdirTemp("", "skillxp-*")
	if err != nil {
		return nil, err
	}
	if !cfg.KeepFixture {
		defer func() { _ = os.RemoveAll(fixture) }()
	} else {
		cfg.logf("fixture kept at %s", fixture)
	}
	project := filepath.Join(fixture, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# skillxp fixture\n"), 0o644); err != nil {
		return nil, err
	}
	for _, dir := range spec.SkillDirs {
		if err := p.InstallSkill(project, dir); err != nil {
			return nil, fmt.Errorf("observe: installing %s: %w", dir, err)
		}
	}
	for _, dir := range spec.ProjectDirs {
		// InstallSkillTo is a plain tree copy into destDir/basename; the
		// project root is just another destination.
		if err := p.InstallSkillTo(project, dir); err != nil {
			return nil, fmt.Errorf("observe: installing project tree %s: %w", dir, err)
		}
	}
	for _, dir := range spec.OverlayDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("observe: overlay %s: %w", dir, err)
		}
		for _, e := range entries {
			if err := p.InstallSkillTo(project, filepath.Join(dir, e.Name())); err != nil {
				return nil, fmt.Errorf("observe: overlaying %s: %w", filepath.Join(dir, e.Name()), err)
			}
		}
	}

	var sb *profile.Sandbox
	if cfg.Sandbox {
		sb, err = p.PrepareSandbox(fixture)
		if err != nil {
			return nil, err
		}
	}
	if len(spec.UserSkillDirs) > 0 {
		if sb.UserSkillDir == "" {
			return nil, fmt.Errorf("observe: %s's sandbox does not model a user skill scope yet", harnessID)
		}
		for _, dir := range spec.UserSkillDirs {
			if err := p.InstallSkillTo(sb.UserSkillDir, dir); err != nil {
				return nil, fmt.Errorf("observe: installing %s at user scope: %w", dir, err)
			}
		}
	}

	so := &SessionObservation{Harness: string(harnessID)}
	if cfg.KeepFixture {
		so.ProjectDir = project
	}
	for i, turn := range spec.Turns {
		if turn.Before != nil {
			if err := turn.Before(project); err != nil {
				return nil, fmt.Errorf("observe: turn %d before-hook: %w", i+1, err)
			}
		}
		obs, err := runTurn(ctx, cfg, p, cliVersion, project, turn, i, so.SessionID, sb)
		if err != nil {
			return nil, fmt.Errorf("observe: turn %d: %w", i+1, err)
		}
		if i == 0 {
			if obs.SessionID == "" {
				return nil, fmt.Errorf("observe: opening turn yielded no session id; cannot resume")
			}
			so.SessionID = obs.SessionID
		}
		so.Turns = append(so.Turns, obs)
	}

	if cfg.ArchiveDir != "" {
		if err := so.Final().archive(cfg.ArchiveDir); err != nil {
			return so, fmt.Errorf("observe: archive: %w", err)
		}
	}
	return so, nil
}

// runTurn invokes one turn and returns its observation. For follow-up
// turns it waits until the (shared, appended) transcript actually records
// the turn's prompt: locate-by-ID succeeds as soon as the opening turn's
// file exists, which can be before the resumed turn flushes.
func runTurn(ctx context.Context, cfg Config, p profile.Profile, cliVersion, project string, turn Turn, index int, sessionID string, sb *profile.Sandbox) (*Observation, error) {
	req := agentsummons.Request{Harness: p.Harness, Prompt: turn.Prompt, Workdir: project}
	if index == 0 {
		if err := p.Prepare(&req, turn.Activation); err != nil {
			return nil, err
		}
	} else {
		p.PrepareResume(&req, turn.Activation, sessionID)
	}
	root := ""
	if sb != nil {
		req.ExtraEnv = append(req.ExtraEnv, sb.Env...)
		root = sb.TranscriptRoot
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cfg.logf("[%s] turn %d: invoking", p.Harness, index+1)
	res, err := invoker.Run(runCtx, req)
	if err != nil {
		return nil, fmt.Errorf("invoke: %w", err)
	}

	var ref harness.SessionRef
	if index == 0 {
		ref, err = p.Locate(ctx, res, root)
	} else {
		ref, err = p.LocateByID(ctx, sessionID, root)
	}
	if err != nil {
		return nil, fmt.Errorf("locate: %w", err)
	}

	sess, err := parseWhenTurnRecorded(ctx, p, cliVersion, ref.Path, turn.Prompt)
	if err != nil {
		return nil, err
	}

	obs := &Observation{
		Harness:        string(p.Harness),
		CLIVersion:     cliVersion,
		HarnessVersion: sess.Meta.HarnessVersion,
		SessionID:      sess.Meta.SessionID,
		Sandboxed:      sb != nil,
		Prompt:         turn.Prompt,
		Activation:     turn.Activation,
		Started:        res.Start,
		Ended:          res.End,
		ExitCode:       res.ExitCode,
		TranscriptPath: ref.Path,
		SubagentPaths:  ref.SubagentPaths,
		Session:        sess,
		Profile:        p,
		Result:         res,
		Ref:            ref,
	}
	if obs.HarnessVersion == "" {
		obs.HarnessVersion = cliVersion
	}
	if obs.SessionID == "" {
		// Antigravity records no session id in-band; discovery derives it
		// from the conversation directory, so the scan ref is the source.
		obs.SessionID = ref.Meta.SessionID
	}
	obs.Model = trace.Model(sess)
	return obs, nil
}

// parseWhenTurnRecorded parses the transcript, retrying until it records
// the turn's prompt as a human message or the deadline passes. Mid-write
// parse errors are retried on the same clock.
func parseWhenTurnRecorded(ctx context.Context, p profile.Profile, cliVersion, path, prompt string) (*session.Session, error) {
	deadline := time.Now().Add(15 * time.Second)
	for {
		sess, err := parseTranscript(p, cliVersion, path)
		if err == nil && hasHumanPrompt(sess, prompt) {
			return sess, nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return nil, fmt.Errorf("parse: %w", err)
			}
			return nil, fmt.Errorf("parse: transcript %s never recorded the turn's prompt", path)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func parseTranscript(p profile.Profile, cliVersion, path string) (*session.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return agentminutes.Parse(f, harness.ID(p.Harness), p.ParseOptions(cliVersion))
}

func hasHumanPrompt(s *session.Session, prompt string) bool {
	for i := range s.Events {
		ev := &s.Events[i]
		if ev.Kind == session.KindUserMessage && ev.UserMessage.Origin == session.OriginHuman &&
			strings.Contains(ev.UserMessage.Text(), prompt) {
			return true
		}
	}
	return false
}

// archive copies the transcript(s) into dir and repoints the observation's
// paths at the copies.
func (o *Observation) archive(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	main := filepath.Join(dir, filepath.Base(o.Ref.Path))
	if err := copyFile(o.Ref.Path, main); err != nil {
		return err
	}
	o.TranscriptPath = main
	var subs []string
	for _, sub := range o.Ref.SubagentPaths {
		dest := filepath.Join(dir, filepath.Base(sub))
		if err := copyFile(sub, dest); err != nil {
			return err
		}
		subs = append(subs, dest)
	}
	o.SubagentPaths = subs
	return nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}
