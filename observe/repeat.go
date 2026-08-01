package observe

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/agent-ecosystem/agentsummons"
)

// RunError records one failed repetition. Failures are data in rate
// reporting, not aborts: for model-dependent behavior the failure rate is
// part of the observation.
type RunError struct {
	// Run is the 1-based repetition index.
	Run int    `json:"run"`
	Err string `json:"error"`
}

// RepeatObservation is n serialized executions of the same session spec.
type RepeatObservation struct {
	Harness string `json:"harness"`

	// Requested is the repetition count asked for; len(Runs) plus
	// len(Errors) accounts for every attempt.
	Requested int                   `json:"requested"`
	Runs      []*SessionObservation `json:"runs"`
	Errors    []RunError            `json:"errors,omitempty"`
}

// Repeat runs spec n times, each in a fresh fixture and session, strictly
// serialized (transcript attribution and context isolation both require
// it). A failed run is recorded and the sequence continues; only context
// cancellation aborts. When Config.ArchiveDir is set, each run archives
// under a run-NN/ subdirectory.
func Repeat(ctx context.Context, cfg Config, harnessID agentsummons.ID, spec SessionSpec, n int) (*RepeatObservation, error) {
	if n < 1 {
		return nil, fmt.Errorf("observe: repeat count must be positive, got %d", n)
	}
	ro := &RepeatObservation{Harness: string(harnessID), Requested: n}
	for i := 1; i <= n; i++ {
		runCfg := cfg
		if cfg.ArchiveDir != "" {
			runCfg.ArchiveDir = filepath.Join(cfg.ArchiveDir, fmt.Sprintf("run-%02d", i))
		}
		cfg.logf("[%s] run %d/%d", harnessID, i, n)
		so, err := ObserveSession(ctx, runCfg, harnessID, spec)
		if err != nil {
			if ctx.Err() != nil {
				return ro, ctx.Err()
			}
			ro.Errors = append(ro.Errors, RunError{Run: i, Err: err.Error()})
			continue
		}
		ro.Runs = append(ro.Runs, so)
	}
	return ro, nil
}

// Tally counts successful runs by the label classify assigns to each —
// the rate-reporting primitive. What the labels mean is the caller's
// business; skillxp only counts.
func Tally(runs []*SessionObservation, classify func(*SessionObservation) string) map[string]int {
	t := make(map[string]int, len(runs))
	for _, r := range runs {
		t[classify(r)]++
	}
	return t
}
