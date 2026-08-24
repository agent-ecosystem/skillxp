package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-ecosystem/agentminutes/session"
	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/observe"
	"github.com/agent-ecosystem/skillxp/profile"
	"github.com/agent-ecosystem/skillxp/trace"
)

func TestRunDispatch(t *testing.T) {
	if err := run(nil); err == nil {
		t.Error("no command: want error")
	}
	if err := run([]string{"frobnicate"}); err == nil {
		t.Error("unknown command: want error")
	}
	for _, args := range [][]string{
		{"version"},
		{"-version"},
		{"--version"},
		{"help"},
		{"-h"},
		{"--help"},
		{"harnesses"},
	} {
		if err := run(args); err != nil {
			t.Errorf("run(%v): %v", args, err)
		}
	}
}

func TestObserveFlagValidation(t *testing.T) {
	skill := filepath.Join(t.TempDir(), "my-skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notASkill := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing required flags", []string{}, "required"},
		{"missing prompt", []string{"-harness", "claude-code", "-install", skill}, "required"},
		{"zero runs", []string{"-harness", "claude-code", "-install", skill, "-prompt", "hi", "-runs", "0"}, "-runs"},
		{"unknown harness", []string{"-harness", "nope", "-install", skill, "-prompt", "hi"}, "unsupported harness"},
		{"install dir without SKILL.md", []string{"-harness", "claude-code", "-install", notASkill, "-prompt", "hi"}, "skill directory"},
		{"user install without SKILL.md", []string{"-harness", "claude-code", "-install", skill, "-install-user", notASkill, "-prompt", "hi", "-sandbox"}, "skill directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := observeCmd(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSummarizeTraces(t *testing.T) {
	perRun := []map[string][]trace.Occurrence{
		{ // run 1: CANARY injected twice (counts once) and echoed
			"CANARY": {
				{Location: trace.LocHarnessInjected},
				{Location: trace.LocHarnessInjected},
				{Location: trace.LocEcho},
			},
		},
		{ // run 2: CANARY absent
			"CANARY": nil,
		},
		{ // run 3: CANARY injected
			"CANARY": {{Location: trace.LocHarnessInjected}},
		},
	}
	got := summarizeTraces(perRun, []string{"CANARY", "GHOST"})
	want := map[string]map[string]int{
		"CANARY": {"harness-injected": 2, "echo": 1, "absent": 1},
		"GHOST":  {"absent": 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("summary = %v, want %v", got, want)
	}

	if got := summarizeTraces(perRun, nil); got != nil {
		t.Errorf("no markers: summary = %v, want nil", got)
	}
}

func TestWriteBundle(t *testing.T) {
	p, err := profile.For(agentsummons.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	obs := &observe.Observation{
		Harness:    "claude-code",
		CLIVersion: "2.1.205",
		Prompt:     "hi",
		Profile:    p,
		Session: &session.Session{Events: []session.Event{
			{Kind: session.KindSystem, System: &session.SystemEvent{Subtype: "attachment/skill_listing", Text: "skills: CANARY"}},
		}},
	}
	dir := t.TempDir()
	occs, err := writeBundle(dir, obs, []string{"CANARY"})
	if err != nil {
		t.Fatal(err)
	}
	if len(occs["CANARY"]) != 1 || occs["CANARY"][0].Location != trace.LocHarnessInjected {
		t.Errorf("traces = %+v", occs)
	}

	data, err := os.ReadFile(filepath.Join(dir, "observation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle struct {
		Harness string                        `json:"harness"`
		Traces  map[string][]trace.Occurrence `json:"traces"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Harness != "claude-code" || len(bundle.Traces["CANARY"]) != 1 {
		t.Errorf("observation.json = %+v", bundle)
	}

	data, err = os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s session.Session
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Events) != 1 {
		t.Errorf("session.json has %d events, want 1", len(s.Events))
	}
}

func TestWriteJSONCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "out.json")
	if err := writeJSON(path, map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("output missing trailing newline")
	}
	var v map[string]int
	if err := json.Unmarshal(data, &v); err != nil || v["n"] != 1 {
		t.Errorf("round-trip = %v, %v", v, err)
	}
}
