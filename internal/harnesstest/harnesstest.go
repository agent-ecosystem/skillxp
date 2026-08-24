// Package harnesstest fakes the claude-code end of the invocation seam for
// tests: its stub writes genuine transcript records into the sandbox's
// store, so the real locate and parse pipeline runs against real files.
package harnesstest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-ecosystem/agentsummons"
)

// User renders a minimal claude-code human user-message record.
func User(sessionID, cwd, prompt, uuid string, ts time.Time) string {
	return fmt.Sprintf(`{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":%q},"uuid":%q,"timestamp":%q,"userType":"external","entrypoint":"cli","cwd":%q,"sessionId":%q,"version":"2.1.205","gitBranch":"main"}`,
		prompt, uuid, ts.UTC().Format("2006-01-02T15:04:05.000Z"), cwd, sessionID) + "\n"
}

// Assistant renders a minimal claude-code assistant-message record.
func Assistant(sessionID, cwd, text, uuid, msgID string, ts time.Time) string {
	return fmt.Sprintf(`{"parentUuid":null,"isSidechain":false,"type":"assistant","message":{"id":%q,"model":"claude-fable-5","role":"assistant","type":"message","stop_reason":"end_turn","content":[{"type":"text","text":%q}],"usage":{"input_tokens":10,"output_tokens":5}},"requestId":"req_1","uuid":%q,"timestamp":%q,"userType":"external","entrypoint":"cli","cwd":%q,"sessionId":%q,"version":"2.1.205","gitBranch":"main"}`,
		msgID, text, uuid, ts.UTC().Format("2006-01-02T15:04:05.000Z"), cwd, sessionID) + "\n"
}

// SidechainUser renders a subagent (sidechain) user record.
func SidechainUser(sessionID, agentID, cwd, text, uuid string, ts time.Time) string {
	return fmt.Sprintf(`{"parentUuid":null,"isSidechain":true,"agentId":%q,"type":"user","message":{"role":"user","content":%q},"uuid":%q,"timestamp":%q,"userType":"external","cwd":%q,"sessionId":%q,"version":"2.1.205","gitBranch":"main"}`,
		agentID, text, uuid, ts.UTC().Format("2006-01-02T15:04:05.000Z"), cwd, sessionID) + "\n"
}

// ExtraEnv extracts KEY's value from a request's ExtraEnv.
func ExtraEnv(req agentsummons.Request, key string) string {
	for _, e := range req.ExtraEnv {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			return v
		}
	}
	return ""
}

// TranscriptPath is where the fake writes a session's transcript inside a
// sandbox config dir.
func TranscriptPath(configDir, sessionID string) string {
	return filepath.Join(configDir, "projects", "-proj", sessionID+".jsonl")
}

// FakeClaudeCode returns an invocation stub that behaves like a sandboxed
// claude-code run: it appends the turn's records to the session transcript
// inside the request's CLAUDE_CONFIG_DIR and echoes the preset session ID.
// Each call is appended to calls.
func FakeClaudeCode(tb testing.TB, calls *[]agentsummons.Request) func(context.Context, agentsummons.Request) (*agentsummons.Result, error) {
	tb.Helper()
	return func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		*calls = append(*calls, req)
		configDir := ExtraEnv(req, "CLAUDE_CONFIG_DIR")
		if configDir == "" {
			tb.Error("invoke: no CLAUDE_CONFIG_DIR in request env")
		}
		sessionID := req.SessionID
		if req.Resume != "" {
			sessionID = req.Resume
		}
		now := time.Now()
		n := len(*calls)
		record := User(sessionID, req.Workdir, req.Prompt, fmt.Sprintf("u-%d", n), now) +
			Assistant(sessionID, req.Workdir, "done", fmt.Sprintf("a-%d", n), fmt.Sprintf("msg_%d", n), now)
		path := TranscriptPath(configDir, sessionID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		if _, err := f.WriteString(record); err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		return &agentsummons.Result{
			Harness:     req.Harness,
			Argv:        []string{"claude", "-p", req.Prompt},
			PromptIndex: 2,
			Workdir:     req.Workdir,
			Start:       now,
			End:         now.Add(time.Second),
			ExitCode:    0,
			SessionID:   req.SessionID,
		}, nil
	}
}

// WriteSkill creates a minimal skill directory named my-skill and returns
// its path.
func WriteSkill(tb testing.TB) string {
	tb.Helper()
	src := filepath.Join(tb.TempDir(), "my-skill")
	if err := os.MkdirAll(src, 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: my-skill\n---\nbody\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	return src
}
