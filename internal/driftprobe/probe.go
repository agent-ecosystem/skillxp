// Package driftprobe is the maintainer-side drift checker behind
// `skillxp drift probe`. It re-runs, against the installed release of each
// harness, the skill-loading experiments the harness profile's lore was
// established on, and grades whether that lore still holds: where skills
// are discovered at project and user scope, whether the discovery listing
// still surfaces where the profile says, and whether resumed turns still
// land in the opening turn's transcript.
//
// The shape, vocabulary, and exit codes mirror agentminutes' drift
// devtool so the agent-ecosystem libraries reconcile drift the same way:
// a version gate against LastValidated, fixed probes, a one-shot retry
// before a probe is called inconclusive, and "inconclusive" kept distinct
// from "drift" so green always means the lore was exercised.
package driftprobe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-ecosystem/agentminutes/session"
	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/internal/invoker"
	"github.com/agent-ecosystem/skillxp/observe"
	"github.com/agent-ecosystem/skillxp/profile"
	"github.com/agent-ecosystem/skillxp/trace"
)

// Category grades a probe outcome. Order is severity (worst wins when
// aggregating); the CLI exit code is a separate mapping.
type Category int

// Outcome categories.
const (
	// Clean means every probe exercised its lore and it held (or the
	// version gate skipped the work).
	Clean Category = iota

	// Inconclusive means the skill was discovered but its body never
	// reached the model even after the retry, so the run proves nothing
	// about loading: the model may simply not have activated it.
	Inconclusive

	// ExecError means the check itself could not run (harness missing or
	// failing, sandbox not seeded, nonzero exit).
	ExecError

	// Drift means the profile's lore did not hold: the skill was absent
	// from the discovery listing, the transcript could not be located or
	// parsed where the profile expects, or a resumed turn landed elsewhere.
	Drift
)

// ExitCode maps a category onto the drift command's documented exit codes:
// 0 clean, 1 drift, 2 inconclusive, 3 execution error.
func (c Category) ExitCode() int {
	switch c {
	case Drift:
		return 1
	case Inconclusive:
		return 2
	case ExecError:
		return 3
	default:
		return 0
	}
}

func (c Category) String() string {
	switch c {
	case Clean:
		return "clean"
	case Inconclusive:
		return "inconclusive"
	case ExecError:
		return "execution error"
	case Drift:
		return "drift"
	}
	return fmt.Sprintf("Category(%d)", int(c))
}

func maxCategory(a, b Category) Category {
	if b > a {
		return b
	}
	return a
}

// Probe is one skill-loading experiment. Each probe stages a fresh
// canary-bearing skill, activates it with the profile's activation
// prompt, and checks the profile's lore against the transcript.
type Probe struct {
	Name string

	// UserScope installs the skill at the sandbox's user scope instead of
	// the fixture's project scope.
	UserScope bool

	// Warmup, when set, opens the session with this passive turn so the
	// activation turn is a resumed one, exercising the profile's
	// resume-attribution lore.
	Warmup string
}

// DefaultProbes covers every piece of lore a profile encodes: project
// scope discovery, user scope discovery, and resume attribution (which
// also re-checks project scope). Listing and echo subtypes are checked on
// every probe.
func DefaultProbes() []Probe {
	return []Probe{
		{Name: "project"},
		{Name: "user", UserScope: true},
		{Name: "resume", Warmup: "Reply with exactly one word: ready"},
	}
}

// DefaultTimeout is the per-invocation timeout used when Options leaves
// Timeout zero (the CLI flag default references it too).
const DefaultTimeout = 5 * time.Minute

// Options configure a probe run.
type Options struct {
	// Force probes even when the installed version equals LastValidated.
	Force bool

	// Keep archives every probe's transcripts under a reported directory.
	Keep bool

	// Timeout bounds each harness invocation; zero means DefaultTimeout.
	Timeout time.Duration

	// MissingBinaryIsError makes an absent harness binary an execution
	// error instead of a skip (set when the harness was selected
	// explicitly rather than defaulted).
	MissingBinaryIsError bool
}

// RunProbes drives each harness through the version gate and the probe
// set, writing a report to w and returning the worst category seen.
// Harnesses run serially: concurrent sessions break transcript attribution.
func RunProbes(ctx context.Context, w io.Writer, ids []agentsummons.ID, probes []Probe, opts Options) Category {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	worst := Clean
	for _, id := range ids {
		worst = maxCategory(worst, runHarness(ctx, w, id, probes, opts))
	}
	return worst
}

func runHarness(ctx context.Context, w io.Writer, id agentsummons.ID, probes []Probe, opts Options) Category {
	say(w, "%s:\n", id)
	p, err := profile.For(id)
	if err != nil {
		say(w, "  error: %v\n", err)
		return ExecError
	}
	installed, err := invoker.Version(ctx, id)
	var nie *agentsummons.NotInstalledError
	if errors.As(err, &nie) {
		if opts.MissingBinaryIsError {
			say(w, "  error: %v\n", err)
			return ExecError
		}
		say(w, "  skipped: harness not installed (%v)\n", err)
		return Clean
	}
	if err != nil {
		say(w, "  error: reading version: %v\n", err)
		return ExecError
	}
	validated := profile.LastValidated[id]
	switch {
	case opts.Force:
		say(w, "  probing %s (last validated %s; --force)\n", installed, validated)
	case agentsummons.VersionNewer(installed, validated):
		say(w, "  probing %s (newer than last validated %s)\n", installed, validated)
	case agentsummons.VersionNewer(validated, installed):
		say(w, "  skipped: installed %s is older than last validated %s\n", installed, validated)
		return Clean
	default:
		say(w, "  up to date: installed %s matches last validated (use --force to probe anyway)\n", installed)
		return Clean
	}

	keepDir := ""
	if opts.Keep {
		dir, err := os.MkdirTemp("", "skillxp-drift-")
		if err != nil {
			say(w, "  error: %v\n", err)
			return ExecError
		}
		keepDir = filepath.Join(dir, string(id))
	}

	cat := Clean
	for i := range probes {
		cat = maxCategory(cat, runOneProbe(ctx, w, p, &probes[i], opts, keepDir))
	}
	switch cat {
	case Clean:
		if installed == validated {
			say(w, "  clean: %s at %s still matches its profile\n", id, installed)
		} else {
			say(w, "  clean: %s at %s still matches its profile; update profile.LastValidated to %s\n", id, installed, installed)
		}
	case Drift:
		say(w, "  to reconcile: re-establish the moved lore in profile.Profiles() (skill directories, listing and echo subtypes, transcript attribution), then update profile.LastValidated and note it in CHANGELOG.md\n")
	case Inconclusive:
		say(w, "  inconclusive: rerun with --force, or inspect a kept transcript (--keep)\n")
	}
	if keepDir != "" {
		if _, err := os.Stat(keepDir); err == nil {
			say(w, "  kept transcripts under %s\n", keepDir)
		} else {
			say(w, "  no transcripts kept (nothing captured, or keeping failed; see above)\n")
		}
	}
	return cat
}

// runOneProbe stages the probe's skill, runs the session, and grades the
// lore. Listing and attribution failures are drift; a body that never
// reaches the model is retried once with an insistent prompt and then
// reported inconclusive.
func runOneProbe(ctx context.Context, w io.Writer, p profile.Profile, probe *Probe, opts Options, keepDir string) Category {
	name := "drift-" + probe.Name + "-skill"
	// A phrase no model produces unprompted; it only reaches the
	// transcript if the skill body did.
	canary := "quartz-lantern-41-" + probe.Name
	skillDir, err := writeSkill(name, canary)
	if err != nil {
		say(w, "  probe %s: error: staging skill: %v\n", probe.Name, err)
		return ExecError
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(skillDir)) }()

	so, cat := runSession(ctx, w, p, probe, opts, keepDir, skillDir, name, p.ActivationPrompt(name), "")
	if cat != Clean {
		return cat
	}
	final := so.Final()
	if drift := loreDrift(p, probe, so, name); len(drift) > 0 {
		for _, d := range drift {
			say(w, "  probe %s: drift: %s\n", probe.Name, d)
		}
		return Drift
	}

	occs := loadingEvidence(trace.Phrase(final.Session, canary, p.EchoSubtypes))
	if len(occs) == 0 {
		say(w, "  probe %s: skill body never reached the model; retrying once with an insistent prompt\n", probe.Name)
		retry, cat := runSession(ctx, w, p, probe, opts, keepDir, skillDir, name, retryPrompt(name), "-retry")
		if cat != Clean {
			return cat
		}
		final = retry.Final()
		occs = loadingEvidence(trace.Phrase(final.Session, canary, p.EchoSubtypes))
		if len(occs) == 0 {
			say(w, "  probe %s: inconclusive: discovered but the body never reached the model (model=%s)\n", probe.Name, final.Model)
			return Inconclusive
		}
	}
	say(w, "  probe %s: discovered and loaded; body reached the model via %s\n", probe.Name, describe(occs))
	return Clean
}

// runSession runs one observed session for the probe and classifies any
// failure: the profile's transcript lore not holding is drift, anything
// else is an execution error.
func runSession(ctx context.Context, w io.Writer, p profile.Profile, probe *Probe, opts Options, keepDir, skillDir, name, prompt, tag string) (*observe.SessionObservation, Category) {
	spec := observe.SessionSpec{}
	if probe.UserScope {
		spec.UserSkillDirs = []string{skillDir}
	} else {
		spec.SkillDirs = []string{skillDir}
	}
	if probe.Warmup != "" {
		spec.Turns = append(spec.Turns, observe.Turn{Prompt: probe.Warmup})
	}
	spec.Turns = append(spec.Turns, observe.Turn{Prompt: prompt, Activation: true})

	cfg := observe.Config{
		Timeout: opts.Timeout,
		Sandbox: true,
		Log:     func(format string, args ...any) { say(w, "    "+format+"\n", args...) },
	}
	if keepDir != "" {
		cfg.ArchiveDir = filepath.Join(keepDir, probe.Name+tag)
	}
	so, err := observe.ObserveSession(ctx, cfg, p.Harness, spec)
	if err != nil {
		var te *observe.TranscriptError
		if errors.As(err, &te) {
			say(w, "  probe %s%s: drift: transcript lore did not hold: %v\n", probe.Name, tag, err)
			return nil, Drift
		}
		say(w, "  probe %s%s: error: %v\n", probe.Name, tag, err)
		return nil, ExecError
	}
	if code := so.Final().ExitCode; code != 0 {
		say(w, "  probe %s%s: error: harness exit code %d\n", probe.Name, tag, code)
		return nil, ExecError
	}
	return so, Clean
}

// loreDrift checks the lore a successful session can still contradict:
// the discovery listing (on harnesses that record one) must name the
// skill, and a resumed session must carry every turn.
func loreDrift(p profile.Profile, probe *Probe, so *observe.SessionObservation, name string) []string {
	var drift []string
	final := so.Final()
	if len(p.SkillListingSubtypes) > 0 && trace.SkillListing(final.Session, p.SkillListingSubtypes, name) < 0 {
		drift = append(drift, fmt.Sprintf("skill %s absent from the discovery listing (%s)", name, strings.Join(p.SkillListingSubtypes, ", ")))
	}
	if probe.Warmup != "" {
		for i, turn := range so.Turns {
			if !hasHumanPrompt(final.Session, turn.Prompt) {
				drift = append(drift, fmt.Sprintf("resumed session %s does not record turn %d's prompt", final.SessionID, i+1))
			}
		}
	}
	return drift
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

// loadingEvidence drops the locations that never count as loading: the
// prompt itself and echo subtypes.
func loadingEvidence(occs []trace.Occurrence) []trace.Occurrence {
	var out []trace.Occurrence
	for _, o := range occs {
		if o.Location == trace.LocHumanPrompt || o.Location == trace.LocEcho {
			continue
		}
		out = append(out, o)
	}
	return out
}

// describe lists the distinct locations in first-seen order.
func describe(occs []trace.Occurrence) string {
	seen := map[trace.Location]bool{}
	var locs []string
	for _, o := range occs {
		if !seen[o.Location] {
			seen[o.Location] = true
			locs = append(locs, string(o.Location))
		}
	}
	return strings.Join(locs, ", ")
}

// retryPrompt is the insistent activation request used once before a
// probe is called inconclusive.
func retryPrompt(skill string) string {
	return fmt.Sprintf("You must activate the %s skill now: use your skill tool or read its SKILL.md, then follow its instructions exactly. Do not answer without doing so.", skill)
}

// writeSkill stages a minimal skill whose body carries the canary and
// returns its directory (inside a fresh temp parent).
func writeSkill(name, canary string) (string, error) {
	parent, err := os.MkdirTemp("", "skillxp-drift-skill-")
	if err != nil {
		return "", err
	}
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body := "---\nname: " + name + "\ndescription: Reports the current codeword when activated.\n---\n\nThe codeword is " + canary + ". State it plainly.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
