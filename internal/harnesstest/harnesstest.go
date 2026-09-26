// Package harnesstest fakes the harness end of the invocation seam for
// tests: its stubs (claude-code and copilot) write genuine transcript
// records into the sandbox's store, so the real locate and parse pipeline
// runs against real files.
package harnesstest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// SkillListing renders a claude-code attachment/skill_listing record naming
// the given skills, the discovery evidence a real session records.
func SkillListing(sessionID, cwd string, names []string, uuid string, ts time.Time) string {
	content := "Available skills: " + strings.Join(names, ", ")
	return fmt.Sprintf(`{"parentUuid":null,"isSidechain":false,"attachment":{"type":"skill_listing","content":%q},"type":"attachment","uuid":%q,"timestamp":%q,"userType":"external","entrypoint":"cli","cwd":%q,"sessionId":%q,"version":"2.1.205","gitBranch":"main"}`,
		content, uuid, ts.UTC().Format("2006-01-02T15:04:05.000Z"), cwd, sessionID) + "\n"
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

// Skill is one skill directory the fake discovered.
type Skill struct {
	Name string
	Body string // SKILL.md contents
	Path string // SKILL.md path
}

// Behavior configures FakeClaudeCodeWith.
type Behavior struct {
	// Listing emits an attachment/skill_listing record naming every skill
	// the fake discovers, the way a real session records discovery. The
	// fake discovers exactly what claude-code would: .claude/skills under
	// the workdir and skills/ under the sandbox's CLAUDE_CONFIG_DIR.
	Listing bool

	// Reply produces the assistant's text for a turn, given the skills
	// discovered; nil answers "done".
	Reply func(req agentsummons.Request, skills []Skill) string

	// OmitBundledFiles makes the fake copilot's delivery wrapper leave out
	// the "Related files" list, the lore drift the probe must catch.
	OmitBundledFiles bool
}

// ActivateReply answers with the body of the discovered skill the prompt
// names, so a phrase seeded in the skill body reaches model output the
// way an activated skill's does; otherwise "done".
func ActivateReply(req agentsummons.Request, skills []Skill) string {
	for _, s := range skills {
		if strings.Contains(req.Prompt, s.Name) {
			return "Activated " + s.Name + ": " + s.Body
		}
	}
	return "done"
}

// FakeClaudeCode returns an invocation stub that behaves like a sandboxed
// claude-code run: it appends the turn's records to the session transcript
// inside the request's CLAUDE_CONFIG_DIR and echoes the preset session ID.
// Each call is appended to calls. The assistant always answers "done" and
// no listing is recorded; see FakeClaudeCodeWith.
func FakeClaudeCode(tb testing.TB, calls *[]agentsummons.Request) func(context.Context, agentsummons.Request) (*agentsummons.Result, error) {
	tb.Helper()
	return FakeClaudeCodeWith(tb, calls, Behavior{})
}

// FakeClaudeCodeWith is FakeClaudeCode with configurable discovery and
// reply behavior.
func FakeClaudeCodeWith(tb testing.TB, calls *[]agentsummons.Request, b Behavior) func(context.Context, agentsummons.Request) (*agentsummons.Result, error) {
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
		skills := discoverSkills(filepath.Join(req.Workdir, ".claude", "skills"), filepath.Join(configDir, "skills"))
		record := User(sessionID, req.Workdir, req.Prompt, fmt.Sprintf("u-%d", n), now)
		if b.Listing {
			names := make([]string, len(skills))
			for i, s := range skills {
				names[i] = s.Name
			}
			record += SkillListing(sessionID, req.Workdir, names, fmt.Sprintf("att-%d", n), now)
		}
		reply := "done"
		if b.Reply != nil {
			reply = b.Reply(req, skills)
		}
		record += Assistant(sessionID, req.Workdir, reply, fmt.Sprintf("a-%d", n), fmt.Sprintf("msg_%d", n), now)
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

// discoverSkills lists the skill directories (those holding a SKILL.md)
// under the given roots, sorted by name. Missing roots contribute nothing.
func discoverSkills(roots ...string) []Skill {
	var skills []Skill
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			body, err := os.ReadFile(filepath.Join(root, e.Name(), "SKILL.md"))
			if err != nil {
				continue
			}
			skills = append(skills, Skill{Name: e.Name(), Body: string(body), Path: filepath.Join(root, e.Name(), "SKILL.md")})
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills
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

// copilotStamp renders a timestamp the way copilot records them.
func copilotStamp(ts time.Time) string {
	return ts.UTC().Format("2006-01-02T15:04:05.000Z")
}

// CopilotStart renders a copilot session.start record, the head every
// transcript opens with (session id, version, cwd).
func CopilotStart(sessionID, cwd, id string, ts time.Time) string {
	return fmt.Sprintf(`{"type":"session.start","data":{"sessionId":%q,"version":1,"producer":"copilot-agent","copilotVersion":"1.0.88","startTime":%q,"context":{"cwd":%q}},"id":%q,"timestamp":%q,"parentId":null}`,
		sessionID, copilotStamp(ts), cwd, id, copilotStamp(ts)) + "\n"
}

// CopilotUser renders a copilot human user.message record.
func CopilotUser(prompt, id, msgID string, ts time.Time) string {
	return fmt.Sprintf(`{"type":"user.message","data":{"content":%q,"messageId":%q,"interactionId":"int-1","turnId":"0"},"id":%q,"timestamp":%q,"parentId":null}`,
		prompt, msgID, id, copilotStamp(ts)) + "\n"
}

// CopilotSystemPrompt renders a copilot system.message record whose prompt
// carries the <available_skills> listing naming the given skills, the
// discovery evidence a real session records on the opening turn only.
func CopilotSystemPrompt(names []string, id string, ts time.Time) string {
	var b strings.Builder
	b.WriteString("You are a test assistant.\n<skill>\n<available_skills>\n")
	for _, n := range names {
		fmt.Fprintf(&b, "<skill>\n  <name>%s</name>\n  <location>project</location>\n</skill>\n", n)
	}
	b.WriteString("</available_skills>\n</skill>\n")
	return fmt.Sprintf(`{"type":"system.message","data":{"role":"system","content":%q,"interactionId":"int-1"},"id":%q,"timestamp":%q,"parentId":null}`,
		b.String(), id, copilotStamp(ts)) + "\n"
}

// CopilotAssistant renders a copilot assistant.message record with text
// and no tool requests.
func CopilotAssistant(text, id, msgID string, ts time.Time) string {
	return fmt.Sprintf(`{"type":"assistant.message","data":{"messageId":%q,"originatingMessageId":"um-1","model":"claude-sonnet-5","content":%q,"toolRequests":[],"interactionId":"int-1","turnId":"0"},"id":%q,"timestamp":%q,"parentId":null}`,
		msgID, text, id, copilotStamp(ts)) + "\n"
}

// CopilotSkillInvoked renders a copilot skill.invoked record, the record a
// real `skill` tool call leaves on a skill's first activation: the
// SKILL.md body with its frontmatter stripped, as the harness hands it to
// the model.
func CopilotSkillInvoked(s Skill, id string, ts time.Time) string {
	return fmt.Sprintf(`{"type":"skill.invoked","data":{"name":%q,"path":%q,"content":%q,"allowedTools":[],"source":"project","trigger":"agent-invoked","model":"claude-sonnet-5","invokedAtTurn":0},"id":%q,"timestamp":%q,"parentId":null}`,
		s.Name, s.Path, stripFrontmatter(s.Body), id, copilotStamp(ts)) + "\n"
}

// CopilotSkillInvokedRef renders a copilot skill.invoked_ref record, what a
// repeat activation of an unchanged body leaves in place of skill.invoked:
// the content's hash and length, no body.
func CopilotSkillInvokedRef(s Skill, id string, ts time.Time) string {
	body := stripFrontmatter(s.Body)
	return fmt.Sprintf(`{"type":"skill.invoked_ref","data":{"name":%q,"path":%q,"allowedTools":[],"source":"project","trigger":"agent-invoked","model":"claude-sonnet-5","invokedAtTurn":0,"contentId":%q,"contentLength":%d},"id":%q,"timestamp":%q,"parentId":null}`,
		s.Name, s.Path, copilotContentID(body), len(body), id, copilotStamp(ts)) + "\n"
}

// CopilotSkillDelivered renders the skill.context_delivered_ref record that
// follows every activation: the SHA-256 of the delivered body and the
// <skill-context> wrapper around it, whose prefix states the skill's base
// directory and, unless omitFiles, lists every file under the skill
// directory other than SKILL.md, the way copilot 1.0.88 composes it.
func CopilotSkillDelivered(s Skill, omitFiles bool, id string, ts time.Time) string {
	dir := filepath.Dir(s.Path)
	prefix := fmt.Sprintf("<skill-context name=%q>\nBase directory for this skill: %s\n\n", s.Name, dir)
	if files := bundledFiles(dir); len(files) > 0 && !omitFiles {
		prefix += "Related files (use view tool to read):\n"
		for _, f := range files {
			prefix += "  - " + f + "\n"
		}
		prefix += "\n"
	}
	return fmt.Sprintf(`{"type":"skill.context_delivered_ref","data":{"interactionId":"int-1","source":%q,"contentId":%q,"prefix":%q,"suffix":"\n</skill-context>"},"id":%q,"timestamp":%q,"parentId":null}`,
		"skill-"+s.Name, copilotContentID(stripFrontmatter(s.Body)), prefix, id, copilotStamp(ts)) + "\n"
}

// copilotContentID is the contentId form copilot records for a body.
func copilotContentID(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// bundledFiles lists every file under dir other than SKILL.md, recursively,
// as absolute paths in walk order.
func bundledFiles(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == filepath.Join(dir, "SKILL.md") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}

// stripFrontmatter drops a leading YAML frontmatter block.
func stripFrontmatter(body string) string {
	rest, ok := strings.CutPrefix(body, "---\n")
	if !ok {
		return body
	}
	_, after, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return body
	}
	return strings.TrimLeft(after, "\n")
}

// CopilotTranscriptPath is where the fake writes a session's transcript
// inside a sandbox COPILOT_HOME.
func CopilotTranscriptPath(home, sessionID string) string {
	return filepath.Join(home, "session-state", sessionID, "events.jsonl")
}

// FakeCopilotWith returns an invocation stub that behaves like a sandboxed
// copilot run: it appends the turn's records to the session transcript
// inside the request's COPILOT_HOME and echoes the preset session ID
// (resume keeps it). Discovery matches copilot's native locations,
// .github/skills under the workdir and skills/ under COPILOT_HOME, and the
// listing (when enabled) is recorded only on an opening turn, the way a
// resumed process reuses its state. When Reply is set and the prompt names
// a discovered skill, the fake also records the delivery a real `skill`
// tool call leaves, so the body is harness-injected evidence the way it is
// on the real harness: skill.invoked with the body the first time a
// session delivers that content, skill.invoked_ref (hash only) on a repeat,
// each followed by the skill.context_delivered_ref wrapper record.
func FakeCopilotWith(tb testing.TB, calls *[]agentsummons.Request, b Behavior) func(context.Context, agentsummons.Request) (*agentsummons.Result, error) {
	tb.Helper()
	delivered := map[string]bool{} // session id + content id
	return func(ctx context.Context, req agentsummons.Request) (*agentsummons.Result, error) {
		*calls = append(*calls, req)
		home := ExtraEnv(req, "COPILOT_HOME")
		if home == "" {
			tb.Error("invoke: no COPILOT_HOME in request env")
		}
		sessionID := req.SessionID
		if req.Resume != "" {
			sessionID = req.Resume
		}
		now := time.Now()
		n := len(*calls)
		skills := discoverSkills(filepath.Join(req.Workdir, ".github", "skills"), filepath.Join(home, "skills"))
		record := ""
		if req.Resume == "" {
			record += CopilotStart(sessionID, req.Workdir, fmt.Sprintf("s-%d", n), now)
		}
		record += CopilotUser(req.Prompt, fmt.Sprintf("u-%d", n), fmt.Sprintf("um-%d", n), now)
		if b.Listing && req.Resume == "" {
			names := make([]string, len(skills))
			for i, s := range skills {
				names[i] = s.Name
			}
			record += CopilotSystemPrompt(names, fmt.Sprintf("sys-%d", n), now)
		}
		reply := "done"
		if b.Reply != nil {
			reply = b.Reply(req, skills)
			for i, s := range skills {
				if !strings.Contains(req.Prompt, s.Name) {
					continue
				}
				key := sessionID + " " + copilotContentID(stripFrontmatter(s.Body))
				if delivered[key] {
					record += CopilotSkillInvokedRef(s, fmt.Sprintf("sk-%d-%d", n, i), now)
				} else {
					delivered[key] = true
					record += CopilotSkillInvoked(s, fmt.Sprintf("sk-%d-%d", n, i), now)
				}
				record += CopilotSkillDelivered(s, b.OmitBundledFiles, fmt.Sprintf("skd-%d-%d", n, i), now)
			}
		}
		record += CopilotAssistant(reply, fmt.Sprintf("a-%d", n), fmt.Sprintf("am-%d", n), now)
		path := CopilotTranscriptPath(home, sessionID)
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
			Argv:        []string{"copilot", "-p", req.Prompt},
			PromptIndex: 2,
			Workdir:     req.Workdir,
			Start:       now,
			End:         now.Add(time.Second),
			ExitCode:    0,
			SessionID:   req.SessionID,
		}, nil
	}
}
