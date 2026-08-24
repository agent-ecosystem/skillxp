package observe

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/agent-ecosystem/agentminutes/harness"
	"github.com/agent-ecosystem/agentminutes/session"
	"github.com/agent-ecosystem/agentsummons"
	"github.com/agent-ecosystem/skillxp/profile"
)

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestObservationJSONContract pins the serialized field names of the
// observation bundle. Graders parse these; renaming a tag is a breaking
// change to every consumer, so it must never happen by accident.
func TestObservationJSONContract(t *testing.T) {
	obs := &Observation{
		Harness:        "claude-code",
		CLIVersion:     "2.1.205",
		HarnessVersion: "2.1.205",
		Model:          "claude-fable-5",
		SessionID:      "s-1",
		Prompt:         "hi",
		Activation:     true,
		Started:        time.Now(),
		Ended:          time.Now(),
		ExitCode:       0,
		Sandboxed:      true,
		TranscriptPath: "/archive/s-1.jsonl",
		SubagentPaths:  []string{"/archive/agent-a1.jsonl"},

		// Excluded from the observation record: Session has its own schema
		// contract and is serialized separately; the rest are in-process
		// handles.
		Session: &session.Session{},
		Profile: profile.Profile{Harness: agentsummons.ClaudeCode},
		Result:  &agentsummons.Result{},
		Ref:     harness.SessionRef{Path: "/native/s-1.jsonl"},
	}
	want := []string{
		"activation", "cli_version", "ended", "exit_code", "harness",
		"harness_version", "model", "prompt", "sandboxed", "session_id",
		"started", "subagent_paths", "transcript_path",
	}
	if got := jsonKeys(t, obs); !reflect.DeepEqual(got, want) {
		t.Errorf("Observation JSON keys = %v, want %v", got, want)
	}
}

func TestSessionObservationJSONContract(t *testing.T) {
	so := &SessionObservation{
		Harness:    "claude-code",
		SessionID:  "s-1",
		ProjectDir: "/fixture/project",
		Turns:      []*Observation{{}},
	}
	want := []string{"harness", "project_dir", "session_id", "turns"}
	if got := jsonKeys(t, so); !reflect.DeepEqual(got, want) {
		t.Errorf("SessionObservation JSON keys = %v, want %v", got, want)
	}
}

func TestRepeatObservationJSONContract(t *testing.T) {
	ro := &RepeatObservation{
		Harness:   "claude-code",
		Requested: 2,
		Runs:      []*SessionObservation{{}},
		Errors:    []RunError{{Run: 2, Err: "boom"}},
	}
	want := []string{"errors", "harness", "requested", "runs"}
	if got := jsonKeys(t, ro); !reflect.DeepEqual(got, want) {
		t.Errorf("RepeatObservation JSON keys = %v, want %v", got, want)
	}
	wantErr := []string{"error", "run"}
	if got := jsonKeys(t, ro.Errors[0]); !reflect.DeepEqual(got, wantErr) {
		t.Errorf("RunError JSON keys = %v, want %v", got, wantErr)
	}
}
