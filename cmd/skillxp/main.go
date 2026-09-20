// Command skillxp runs skill invocation experiments against installed
// agent harnesses: install a skill in a fresh fixture, invoke the harness
// headlessly, and report what actually reached the model, with transcript
// evidence. It renders no verdicts; graders (benchmark runners, skill CI)
// consume its observations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/internal/driftprobe"
	"github.com/agent-ecosystem/skillxp/internal/invoker"
	"github.com/agent-ecosystem/skillxp/observe"
	"github.com/agent-ecosystem/skillxp/profile"
	"github.com/agent-ecosystem/skillxp/trace"
)

// version is stamped by goreleaser on release builds (default ldflags,
// -X main.version). Must stay a package-level var named "version" for
// that stamping to land.
var version = "dev"

// cliVersion returns the stamped release version, falling back to the
// module version Go embeds under `go install` (where no ldflags run).
func cliVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			// The report already said everything.
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "skillxp:", err)
		os.Exit(1)
	}
}

// exitError carries a documented non-zero exit code for a command whose
// report already explained the outcome; main exits with it silently.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("a command is required")
	}
	switch args[0] {
	case "harnesses":
		return harnesses(os.Stdout)
	case "doctor":
		return doctor(os.Stdout)
	case "drift":
		// Maintainer tooling, deliberately absent from usage (see
		// DEVELOPMENT.md): it spends real tokens on every probed harness.
		return driftCmd(os.Stdout, args[1:])
	case "observe":
		return observeCmd(args[1:])
	case "version", "-version", "--version":
		fmt.Println("skillxp version", cliVersion())
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage:
  skillxp harnesses
      Show supported harnesses, the harness versions this release's
      skill lore and flag surface were validated against, and their
      skill install locations.

  skillxp doctor
      Compare each installed harness version against the version this
      release's skill lore was validated on (free; nothing is invoked
      beyond the harnesses' version commands). A newer installed release
      is a drift candidate, not a failure. Exit 0 clean, 1 drift
      candidate, 2 a version probe failed.

  skillxp version
      Print the skillxp version.

  skillxp observe -harness <id> -install <skill-dir> -prompt <text> [flags]
      Install skill(s) in a fixture, invoke the harness, and write an
      observation bundle: observation.json (run metadata and trace
      report), session.json (the normalized transcript), and the archived
      native transcript(s). With -runs N, each repetition gets a fresh
      fixture and session under run-NN/, and summary.json reports how
      often each traced phrase reached each location across runs.

Observe flags:
  -harness string    harness id (required; see 'skillxp harnesses')
  -install string    skill directory to install at project scope; comma-separate for several (required)
  -install-user string  skill directory to install at USER scope (requires -sandbox)
  -prompt string     the prompt (required; never include a phrase you plan to trace)
  -activation        request skill-activation permissions for the turn
  -trace string      phrase(s) to trace through the transcript; comma-separated
  -runs int          repetitions, each a fresh fixture and session (default 1)
  -out string        bundle directory (default ./skillxp-out)
  -timeout duration  invocation timeout (default 5m)
  -keep              keep the fixture directory
  -sandbox           run against an isolated home (see README: sandbox seeds)
`)
}

func harnesses(w io.Writer) error {
	for _, p := range profile.Profiles() {
		injected := "records injected context"
		if !p.RecordsInjectedContext {
			injected = "does NOT record injected context (evidence is inference)"
		}
		// Two validation axes, both coverage documentation rather than a
		// compatibility bound (newer harness releases usually keep working):
		// this release's skill lore (profile.LastValidated) and the flag
		// surface it invokes through (agentsummons.LastValidated).
		lore := orUnknown(profile.LastValidated[p.Harness])
		flags := orUnknown(agentsummons.LastValidated[p.Harness])
		if _, err := fmt.Fprintf(w, "%-14s validated %-9s flags %-9s project skills: %-16s %s\n", p.Harness, lore, flags, p.ProjectSkillDir, injected); err != nil {
			return err
		}
	}
	return nil
}

// doctor's exit codes; the worst one seen wins.
const (
	exitDoctorDrift       = 1
	exitDoctorProbeFailed = 2
)

// versionProbeTimeout bounds each harness's version command; these are
// free, local invocations that should never take long.
const versionProbeTimeout = 30 * time.Second

// doctor mirrors `agentsummons doctor` for the skill-lore axis: installed
// version against profile.LastValidated, per harness.
func doctor(w io.Writer) error {
	worst := 0
	for _, p := range profile.Profiles() {
		validated := profile.LastValidated[p.Harness]
		ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
		installed, err := invoker.Version(ctx, p.Harness)
		cancel()
		var nie *agentsummons.NotInstalledError
		switch {
		case errors.As(err, &nie):
			_, _ = fmt.Fprintf(w, "%-12s not installed\n", p.Harness)
		case err != nil:
			_, _ = fmt.Fprintf(w, "%-12s version probe failed: %v\n", p.Harness, err)
			worst = max(worst, exitDoctorProbeFailed)
		case agentsummons.VersionNewer(installed, validated):
			_, _ = fmt.Fprintf(w, "%-12s installed %s > validated %s — drift candidate; run `skillxp drift probe` to revalidate\n", p.Harness, installed, validated)
			worst = max(worst, exitDoctorDrift)
		default:
			_, _ = fmt.Fprintf(w, "%-12s installed %s, validated %s — clean\n", p.Harness, installed, validated)
		}
	}
	if worst != 0 {
		return exitError{worst}
	}
	return nil
}

// driftCmd is `skillxp drift probe`: the maintainer-only revalidation of
// each harness profile's lore against the installed release. Exit codes
// follow agentminutes' drift devtool: 0 clean, 1 drift, 2 inconclusive,
// 3 execution error.
func driftCmd(w io.Writer, args []string) error {
	if len(args) == 0 || args[0] != "probe" {
		return fmt.Errorf("drift: usage: skillxp drift probe [-harness id,...] [-force] [-keep] [-timeout d]")
	}
	fs := flag.NewFlagSet("drift probe", flag.ContinueOnError)
	harnessList := fs.String("harness", "", "harnesses to probe, comma-separated (default all)")
	force := fs.Bool("force", false, "probe even when the installed version equals the last validated one")
	keep := fs.Bool("keep", false, "keep the probes' transcripts for inspection")
	timeout := fs.Duration("timeout", driftprobe.DefaultTimeout, "per-invocation timeout")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var ids []agentsummons.ID
	explicit := *harnessList != ""
	if explicit {
		for _, name := range strings.Split(*harnessList, ",") {
			id := agentsummons.ID(strings.TrimSpace(name))
			if _, err := profile.For(id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
	} else {
		for _, p := range profile.Profiles() {
			ids = append(ids, p.Harness)
		}
	}
	cat := driftprobe.RunProbes(context.Background(), w, ids, driftprobe.DefaultProbes(), driftprobe.Options{
		Force:                *force,
		Keep:                 *keep,
		Timeout:              *timeout,
		MissingBinaryIsError: explicit,
	})
	if code := cat.ExitCode(); code != 0 {
		return exitError{code}
	}
	return nil
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func observeCmd(args []string) error {
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	harnessID := fs.String("harness", "", "harness id")
	install := fs.String("install", "", "skill directories, comma-separated")
	installUser := fs.String("install-user", "", "user-scope skill directories, comma-separated (requires -sandbox)")
	prompt := fs.String("prompt", "", "prompt text")
	activation := fs.Bool("activation", false, "request activation permissions")
	traces := fs.String("trace", "", "phrases to trace, comma-separated")
	runs := fs.Int("runs", 1, "repetitions, each a fresh fixture and session")
	out := fs.String("out", "skillxp-out", "bundle directory")
	timeout := fs.Duration("timeout", 5*time.Minute, "invocation timeout")
	keep := fs.Bool("keep", false, "keep fixture directory")
	sandbox := fs.Bool("sandbox", false, "run against an isolated home")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *harnessID == "" || *install == "" || *prompt == "" {
		return fmt.Errorf("-harness, -install, and -prompt are required")
	}
	if *runs < 1 {
		return fmt.Errorf("-runs must be positive")
	}
	p, err := profile.For(agentsummons.ID(*harnessID))
	if err != nil {
		return err
	}

	spec := observe.SessionSpec{Turns: []observe.Turn{{Prompt: *prompt, Activation: *activation}}}
	for _, dir := range strings.Split(*install, ",") {
		dir = strings.TrimSpace(dir)
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			return fmt.Errorf("%s does not look like a skill directory: %w", dir, err)
		}
		spec.SkillDirs = append(spec.SkillDirs, dir)
	}
	if *installUser != "" {
		for _, dir := range strings.Split(*installUser, ",") {
			dir = strings.TrimSpace(dir)
			if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
				return fmt.Errorf("%s does not look like a skill directory: %w", dir, err)
			}
			spec.UserSkillDirs = append(spec.UserSkillDirs, dir)
		}
	}
	var markers []string
	if *traces != "" {
		for _, m := range strings.Split(*traces, ",") {
			markers = append(markers, strings.TrimSpace(m))
		}
	}

	cfg := observe.Config{
		Timeout:     *timeout,
		Sandbox:     *sandbox,
		KeepFixture: *keep,
		ArchiveDir:  *out,
		Log:         func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) },
	}
	ctx := context.Background()

	if *runs == 1 {
		so, err := observe.ObserveSession(ctx, cfg, p.Harness, spec)
		if err != nil {
			return err
		}
		obs := so.Final()
		occs, err := writeBundle(*out, obs, markers)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s model=%s exit=%d transcript=%s\n", obs.Harness, obs.HarnessVersion, obs.Model, obs.ExitCode, obs.TranscriptPath)
		printTraces(occs, markers)
		return nil
	}

	ro, err := observe.Repeat(ctx, cfg, p.Harness, spec, *runs)
	if err != nil {
		return err
	}
	// Per-run bundles land next to the transcripts Repeat archived.
	perRun := make([]map[string][]trace.Occurrence, 0, len(ro.Runs))
	for i, so := range ro.Runs {
		obs := so.Final()
		occs, err := writeBundle(filepath.Dir(obs.TranscriptPath), obs, markers)
		if err != nil {
			return fmt.Errorf("run %d bundle: %w", i+1, err)
		}
		perRun = append(perRun, occs)
	}

	summary := struct {
		Harness   string             `json:"harness"`
		Requested int                `json:"requested"`
		Succeeded int                `json:"succeeded"`
		Errors    []observe.RunError `json:"errors,omitempty"`
		// Traces counts, per phrase and location, the number of runs in
		// which the phrase appeared at that location at least once;
		// "absent" counts runs where it never appeared.
		Traces map[string]map[string]int `json:"traces,omitempty"`
	}{Harness: ro.Harness, Requested: ro.Requested, Succeeded: len(ro.Runs), Errors: ro.Errors}
	summary.Traces = summarizeTraces(perRun, markers)
	if err := writeJSON(filepath.Join(*out, "summary.json"), summary); err != nil {
		return err
	}

	fmt.Printf("%s: %d/%d runs succeeded\n", ro.Harness, len(ro.Runs), ro.Requested)
	for _, e := range ro.Errors {
		fmt.Printf("run %d failed: %s\n", e.Run, e.Err)
	}
	for _, m := range markers {
		for loc, n := range summary.Traces[m] {
			fmt.Printf("trace %s: %s in %d/%d runs\n", m, loc, n, len(ro.Runs))
		}
	}
	return nil
}

// summarizeTraces counts, per phrase and location, the number of runs in
// which the phrase appeared at that location at least once; "absent" counts
// runs where it never appeared. Nil when no phrases were traced.
func summarizeTraces(perRun []map[string][]trace.Occurrence, markers []string) map[string]map[string]int {
	if len(markers) == 0 {
		return nil
	}
	summary := map[string]map[string]int{}
	for _, m := range markers {
		counts := map[string]int{}
		for _, occs := range perRun {
			locs := map[string]bool{}
			for _, o := range occs[m] {
				locs[string(o.Location)] = true
			}
			if len(locs) == 0 {
				counts["absent"]++
			}
			for loc := range locs {
				counts[loc]++
			}
		}
		summary[m] = counts
	}
	return summary
}

// writeBundle writes observation.json and session.json for one turn's
// observation and returns the trace report it embedded.
func writeBundle(dir string, obs *observe.Observation, markers []string) (map[string][]trace.Occurrence, error) {
	bundle := struct {
		*observe.Observation
		Traces map[string][]trace.Occurrence `json:"traces,omitempty"`
	}{Observation: obs}
	if len(markers) > 0 {
		bundle.Traces = map[string][]trace.Occurrence{}
		for _, m := range markers {
			bundle.Traces[m] = trace.Phrase(obs.Session, m, obs.Profile.EchoSubtypes)
		}
	}
	if err := writeJSON(filepath.Join(dir, "observation.json"), bundle); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(dir, "session.json"), obs.Session); err != nil {
		return nil, err
	}
	return bundle.Traces, nil
}

func printTraces(occs map[string][]trace.Occurrence, markers []string) {
	for _, m := range markers {
		if len(occs[m]) == 0 {
			fmt.Printf("trace %s: absent\n", m)
			continue
		}
		for _, o := range occs[m] {
			fmt.Printf("trace %s: %s (%s, event %d, line %d)\n", m, o.Location, o.Kind, o.EventIndex, o.Line)
		}
	}
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
